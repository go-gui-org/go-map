package mapview

import (
	"time"

	"github.com/go-gui-org/go-gui/gui"
	"github.com/go-gui-org/go-map/projection"
)

// dragThresholdPx is the pixel distance separating a click from a pan.
const dragThresholdPx float32 = 4

// onMouseDown handles mouse-down on the canvas. HUD buttons get a hit
// test first so the recenter button does not trigger a drag-pan;
// otherwise the press is recorded and MouseLock takes over the
// subsequent mouse-move / mouse-up events. The press becomes a pan
// only once the cursor leaves the drag-threshold radius; shorter
// presses collapse into a click at mouse-up time.
func onMouseDown(c Cfg, seed MapState) func(gui.EventCtx) {
	id := c.ID
	return func(ctx gui.EventCtx) {
		// Popup-rect hit-test runs first so a click on the InfoWindow
		// body neither starts a drag-pan nor falls through to an
		// overlay beneath the popup. No-op when no popup is drawn.
		if handlePopupClick(ctx.Window, id, ctx.Event) {
			return
		}
		if homeButtonHit(ctx.Layout.Shape.Width, ctx.Event.MouseX, ctx.Event.MouseY) {
			nsWrite(ctx.Window, nsState, id, seed)
			ctx.Consume()
			return
		}
		// A new press cancels any in-flight kinetic fling — user is
		// taking over. Must precede the nsPan write so a zero-
		// velocity drag does not inherit residual fling state.
		cancelKineticPan(ctx.Window, id)
		s := nsRead[MapState](ctx.Window, nsState, id)
		// OnClick delivers widget-local coords; MouseLock callbacks
		// deliver absolute coords. Storing both, plus canvas size,
		// lets panDragEnd resolve the release LatLng without a second
		// event dispatch.
		now := time.Now()
		nsWrite(ctx.Window, nsPan, id, panState{
			Active:    true,
			StartX:    ctx.Event.MouseX + ctx.Layout.Shape.X,
			StartY:    ctx.Event.MouseY + ctx.Layout.Shape.Y,
			LocalX:    ctx.Event.MouseX,
			LocalY:    ctx.Event.MouseY,
			StartCtr:  s.Center,
			StartZoom: s.Zoom,
			CanvasW:   ctx.Layout.Shape.Width,
			CanvasH:   ctx.Layout.Shape.Height,
			LastX:     ctx.Event.MouseX,
			LastY:     ctx.Event.MouseY,
			LastT:     now,
		})
		ctx.Window.MouseLock(gui.MouseLockCfg{
			MouseMove: panDragMove(id),
			MouseUp:   panDragEnd(c),
		})
		ctx.Consume()
	}
}

func homeButtonHit(canvasW, mx, my float32) bool {
	x, y, w, h := homeButtonRect(canvasW)
	return mx >= x && mx < x+w && my >= y && my < y+h
}

func panDragMove(id string) func(gui.EventCtx) {
	return func(ctx gui.EventCtx) {
		p := nsRead[panState](ctx.Window, nsPan, id)
		if !p.Active {
			return
		}
		// Non-finite coords defeat the threshold check (NaN < x is always
		// false) and would inject NaN into the kinetic velocity EMA.
		if !mousePositionFinite(ctx.Event) {
			ctx.Consume()
			return
		}
		dx := p.StartX - ctx.Event.MouseX
		dy := p.StartY - ctx.Event.MouseY
		if !p.Moved {
			// Swallow intra-threshold movement entirely. Prevents the
			// map from jittering when the user shakes the pointer mid-
			// click, and keeps the drag-vs-click decision crisp.
			if dx*dx+dy*dy < dragThresholdPx*dragThresholdPx {
				ctx.Consume()
				return
			}
			p.Moved = true
			nsWrite(ctx.Window, nsPan, id, p)
		}
		// Sample kinetic-pan velocity before the nsWrite that moves
		// center — the EMA math needs the prior LastX/Y, and the
		// subsequent nsPan write persists both the new sample and
		// the updated EMA.
		sampleKineticVelocity(&p, ctx.Event.MouseX, ctx.Event.MouseY, time.Now())
		nsWrite(ctx.Window, nsPan, id, p)

		startPt := projection.ProjectF(p.StartCtr, p.StartZoom)
		newCtr := projection.UnprojectF(projection.Point{
			X: startPt.X + float64(dx),
			Y: startPt.Y + float64(dy),
		}, p.StartZoom)

		s := nsRead[MapState](ctx.Window, nsState, id)
		s.Center = newCtr.Clamp()
		nsWrite(ctx.Window, nsState, id, s)
		ctx.Consume()
	}
}

// panDragEnd finalises a press-and-release. A drag that never crossed
// the threshold becomes a click: OnPOISelect fires first for the
// top-most overlay under the release point, then Marker.OnClick, then
// Cfg.OnClick. The release point comes from the mouse-up event (in
// absolute window coords) converted back into widget-local coords via
// panState.StartX/Y; within the drag threshold this agrees with the
// press point to within a few pixels.
func panDragEnd(c Cfg) func(gui.EventCtx) {
	id := c.ID
	return func(ctx gui.EventCtx) {
		p := nsRead[panState](ctx.Window, nsPan, id)
		wasClick := p.Active && !p.Moved
		// Try launching a kinetic fling before the pan state clears
		// — spawnKineticPan reads p.VelX/VelY/LastT, so the pre-
		// reset snapshot is what it needs to decide.
		if !wasClick {
			spawnKineticPan(ctx.Window, id, p, time.Now())
		}
		// Clear the entire panState — the drag is done, so no field
		// is meaningful and stale StartCtr / LastT entries would
		// survive in the registry otherwise (cosmetic, but a later
		// reader would have to reason about "is this from the
		// current drag or the last one?").
		nsWrite(ctx.Window, nsPan, id, panState{})
		ctx.Window.MouseUnlock()
		if !wasClick {
			return
		}
		// Up-event coords are absolute (MouseLock delivery convention);
		// shift into widget-local space using the down-event offset.
		upX := ctx.Event.MouseX - (p.StartX - p.LocalX)
		upY := ctx.Event.MouseY - (p.StartY - p.LocalY)
		s := nsRead[MapState](ctx.Window, nsState, id)
		vp := computeViewport(p.CanvasW, p.CanvasH, s)
		// Hit-test once per click. Walking Range forward and keeping
		// the last match makes the topmost (last-drawn) overlay win
		// without needing a reverse iterator — BoundedMap only exposes
		// forward order. Hoist the hit-test above the OnPOISelect-nil
		// gate so a Marker.OnClick still fires when the author elected
		// not to set the map-level selector.
		var hit Overlay
		readOverlays(ctx.Window, id).Range(func(_ string, o Overlay) bool {
			if o.HitTest(vp, upX, upY) {
				hit = o
			}
			return true
		})
		// A marker hit is also a focus event: the clicked marker
		// becomes keyboard-focused and its InfoWindow opens (when
		// Title is set). Mirrors the Enter-on-focused-marker path so
		// click and keyboard converge on the same popup state. A
		// click into empty space (no overlay under the release) with
		// a popup open dismisses the popup — matches the "click
		// outside" gesture the plan lists alongside Escape and the
		// close button. Skip the write when nothing changed so a
		// second click on the already-focused marker, or an idle
		// click over empty water, never thrashes the state map.
		if m, ok := hit.(*Marker); ok {
			wantOpen := s.InfoOpen
			if m.Title != "" {
				wantOpen = true
			}
			// Reset sub-element focus when switching marker or
			// opening a fresh popup; preserve it on a re-click of
			// the already-open marker.
			wantIdx := s.InfoFocusIndex
			if wantOpen && (s.FocusedOverlayID != m.MarkerID || !s.InfoOpen) {
				wantIdx = 0
			}
			if s.FocusedOverlayID != m.MarkerID ||
				s.InfoOpen != wantOpen ||
				s.InfoFocusIndex != wantIdx {
				s.FocusedOverlayID = m.MarkerID
				s.InfoOpen = wantOpen
				s.InfoFocusIndex = wantIdx
				nsWrite(ctx.Window, nsState, id, s)
			}
		} else if hit == nil && s.InfoOpen {
			s.InfoOpen = false
			s.InfoFocusIndex = 0
			nsWrite(ctx.Window, nsState, id, s)
		}
		if hit != nil && c.OnPOISelect != nil {
			c.OnPOISelect(ctx.Window, hit)
		}
		if m, ok := hit.(*Marker); ok && m.OnClick != nil {
			m.OnClick(ctx.Window)
		}
		if c.OnClick != nil {
			c.OnClick(ctx.Window, vp.screenToLatLng(upX, upY))
		}
	}
}

func onMouseMove(id string) func(gui.EventCtx) {
	return func(ctx gui.EventCtx) {
		// Reject non-finite coords — the HUD reads hover state directly and
		// would render "NaN°N" if a garbage sample were stored.
		if !mousePositionFinite(ctx.Event) {
			return
		}
		nsWrite(ctx.Window, nsHover, id, hoverState{X: ctx.Event.MouseX, Y: ctx.Event.MouseY, Valid: true})
	}
}

// onMouseLeave clears the hover so the coord readout falls back to
// the map center, matching the convention "no cursor → center".
func onMouseLeave(id string) func(gui.EventCtx) {
	return func(ctx gui.EventCtx) {
		nsWrite(ctx.Window, nsHover, id, hoverState{})
	}
}
