package mapview

import (
	"math"

	"github.com/go-gui-org/go-gui/gui"
	"github.com/go-gui-org/go-map/projection"
)

// onMouseScroll handles wheel zoom. Positive ScrollY zooms in;
// negative zooms out. The accumulator fires sub-ticks of
// scrollZoomStep so trackpad pixel-scroll produces smooth fractional
// zoom; a notch wheel at the default gain (1.0) lands on integer
// rest states because four sub-ticks sum to one full level. gain
// (from Cfg.ScrollZoomGain) scales ScrollY before it hits the
// accumulator, so gain < 1 yields fractional zoom even on notch
// hardware. Zoom pivots toward the cursor so the LatLng under the
// cursor stays fixed.
func onMouseScroll(id string, gain float32) func(*gui.Layout, *gui.Event, *gui.Window) {
	return func(l *gui.Layout, e *gui.Event, w *gui.Window) {
		if e.ScrollY == 0 {
			return
		}
		// Reject NaN/±Inf scroll deltas before they pollute the
		// accumulator — once stuck, every future event would yield
		// NaN+x = NaN and the wheel would silently stop zooming.
		if !isFiniteF32(e.ScrollY) {
			return
		}
		// Reject non-finite cursor position — zoomToward would forward NaN
		// through UnprojectF, which returns LatLng{}, jumping center to (0,0).
		if !mousePositionFinite(e) {
			return
		}
		acc := nsRead[float32](w, nsScroll, id) + e.ScrollY*gain
		delta, acc := scrollSteps(acc)
		nsWrite(w, nsScroll, id, acc)
		if delta == 0 {
			e.IsHandled = true
			return
		}

		s := nsRead[MapState](w, nsState, id)
		newZoom := clampZoom(s.Zoom + float64(delta))
		if srcMax := float64(baseMaxZoom(w, id)); newZoom > srcMax {
			newZoom = srcMax
		}
		if newZoom == s.Zoom {
			e.IsHandled = true
			return
		}
		// Wheel zoom cancels any in-flight fling — user is
		// actively changing the view, momentum is no longer wanted.
		cancelKineticPan(w, id)
		newCtr := zoomToward(
			s, newZoom,
			e.MouseX, e.MouseY,
			l.Shape.Width, l.Shape.Height,
		)

		s.Center = newCtr
		s.Zoom = newZoom
		nsWrite(w, nsState, id, s)
		e.IsHandled = true
	}
}

// scrollZoomStep is the accumulator threshold at which one zoom
// sub-tick fires. At 0.25 a notch wheel (|ScrollY|≈1 per event) still
// lands on integer zoom after 4 sub-ticks while a trackpad pixel-
// scroll produces smooth fractional zoom per event. Keyboard +/-
// stays at integer deltas (see onKeyDown) so discoverable rest states
// remain reachable without wheel finesse.
const scrollZoomStep float32 = 0.25

// maxScrollAccum bounds the accumulator before it is consumed. A
// runaway scroll event (or many events between consumes) cannot make
// the computed delta dwarf the zoom range — the clampZoom downstream
// binds it to [0, maxZoomF] anyway, so capping is observable only to
// abusive input.
const maxScrollAccum float32 = 64

// scrollSteps consumes zoom sub-ticks from the accumulator and
// returns the fractional delta along with the residual. NaN flushes
// to zero (it cannot drive zoom and would otherwise re-enter the
// accumulator). Magnitudes are capped to maxScrollAccum so a single
// huge ScrollY cannot yield an excessive delta. Pure function —
// Window-free, testable.
func scrollSteps(acc float32) (delta, residual float32) {
	if math.IsNaN(float64(acc)) {
		return 0, 0
	}
	if acc > maxScrollAccum {
		acc = maxScrollAccum
	} else if acc < -maxScrollAccum {
		acc = -maxScrollAccum
	}
	steps := int32(acc / scrollZoomStep)
	delta = float32(steps) * scrollZoomStep
	residual = acc - delta
	return
}

// zoomToward returns the new map center so that the LatLng under the
// cursor at (cx, cy) stays fixed across the zoom transition. widgetW
// and widgetH are the canvas dimensions at the time of the event.
// Pure function — no Window or state-registry access — so the
// invariant is unit-testable. Fractional zoom routes through the
// F-variants; callers guarantee newZoom is clamp-safe.
func zoomToward(
	s MapState, newZoom float64,
	cx, cy, widgetW, widgetH float32,
) projection.LatLng {
	oldCtrPx := projection.ProjectF(s.Center, s.Zoom)
	cursorPxOld := projection.Point{
		X: oldCtrPx.X + float64(cx-widgetW/2),
		Y: oldCtrPx.Y + float64(cy-widgetH/2),
	}
	cursorLL := projection.UnprojectF(cursorPxOld, s.Zoom)
	cursorPxNew := projection.ProjectF(cursorLL, newZoom)
	newCtrPx := projection.Point{
		X: cursorPxNew.X - float64(cx-widgetW/2),
		Y: cursorPxNew.Y - float64(cy-widgetH/2),
	}
	return projection.UnprojectF(newCtrPx, newZoom).Clamp()
}

// baseMaxZoom reports the zoom clamp for wheel / keyboard input: the
// smaller of maxZoom and the base layer's MaxZoom. Reference layers
// silently clip at higher Z; only the base constrains input.
func baseMaxZoom(w *gui.Window, id string) uint32 {
	l, ok := baseLayer(w, id)
	if !ok || l.Source == nil {
		return maxZoom
	}
	if z := l.Source.MaxZoom(); z < maxZoom {
		return z
	}
	return maxZoom
}
