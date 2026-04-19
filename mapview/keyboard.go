package mapview

import (
	"github.com/mike-ward/go-gui/gui"
	"github.com/mike-ward/go-map/projection"
)

func onKeyDown(c Cfg, seed MapState) func(*gui.Layout, *gui.Event, *gui.Window) {
	id := c.ID
	return func(_ *gui.Layout, e *gui.Event, w *gui.Window) {
		s := gui.StateReadOr[string, MapState](w, nsState, id, seed)
		if handleFocusKey(c, &s, e, w) {
			e.IsHandled = true
			return
		}
		step := float64(projection.TileSize) / 2
		switch {
		case e.Modifiers.Has(gui.ModShift):
			step = float64(projection.TileSize)
		case e.Modifiers.Has(gui.ModCtrl):
			step = float64(projection.TileSize) / 4
		}

		handled := true
		switch e.KeyCode {
		case gui.KeyLeft:
			s.Center = shiftCenter(s, -step, 0)
		case gui.KeyRight:
			s.Center = shiftCenter(s, step, 0)
		case gui.KeyUp:
			s.Center = shiftCenter(s, 0, -step)
		case gui.KeyDown:
			s.Center = shiftCenter(s, 0, step)
		case gui.KeyEqual, gui.KeyKPAdd:
			// Integer delta — slice 5a keeps keyboard and wheel on
			// whole-number steps. clampZoom enforces the ceiling;
			// baseMaxZoom adds a tighter per-source cap when set.
			if nz := clampZoom(s.Zoom + 1); nz <= float64(baseMaxZoom(w, id)) {
				s.Zoom = nz
			}
		case gui.KeyMinus, gui.KeyKPSubtract:
			if s.Zoom > 0 {
				s.Zoom = clampZoom(s.Zoom - 1)
			}
		case gui.KeyHome:
			s = seed
		default:
			handled = false
		}
		if handled {
			// Keyboard pan / zoom / home all preempt any in-flight
			// fling — explicit user input trumps momentum.
			cancelKineticPan(w, id)
			nsWrite(w, nsState, id, s)
			e.IsHandled = true
		}
	}
}

// handleFocusKey processes Tab, Enter, and Escape for marker-mode focus
// navigation. Returns true when the event was consumed. Every consumed branch
// writes nsState exactly once so a callback that reads Snapshot sees the
// post-dismissal state. When InfoOpen, Tab/Shift-Tab trap focus inside the
// popup and Enter activates the focused sub-element; marker cycling is blocked.
func handleFocusKey(c Cfg, s *MapState, e *gui.Event, w *gui.Window) bool {
	switch e.KeyCode {
	case gui.KeyTab:
		// Popup focus-trap takes priority. Without this short-circuit
		// Tab would walk to the next marker and close the popup,
		// violating the dialog contract.
		if s.InfoOpen {
			bm := readOverlays(w, c.ID)
			m := focusedMarker(bm, *s)
			if m == nil {
				s.InfoOpen = false
				s.InfoFocusIndex = 0
				nsWrite(w, nsState, c.ID, *s)
				return true
			}
			step := 1
			if e.Modifiers.Has(gui.ModShift) {
				step = -1
			}
			s.InfoFocusIndex = cycleInfoFocus(s.InfoFocusIndex, len(m.Actions), step)
			nsWrite(w, nsState, c.ID, *s)
			return true
		}
		ids := focusableMarkerIDs(readOverlays(w, c.ID))
		if len(ids) == 0 || s.FocusedOverlayID == "" {
			return false
		}
		step := 1
		if e.Modifiers.Has(gui.ModShift) {
			step = -1
		}
		s.FocusedOverlayID = nextFocusID(ids, s.FocusedOverlayID, step)
		nsWrite(w, nsState, c.ID, *s)
		return true
	case gui.KeyEnter:
		bm := readOverlays(w, c.ID)
		if s.InfoOpen {
			m := focusedMarker(bm, *s)
			if m == nil {
				s.InfoOpen = false
				s.InfoFocusIndex = 0
				nsWrite(w, nsState, c.ID, *s)
				return true
			}
			idx := int(s.InfoFocusIndex)
			dispatchInfoAction(w, c.ID, m, idx)
			// Reflect the dismissal in the caller's local snapshot too
			// (tests read s after handleFocusKey returns).
			s.InfoOpen = false
			s.InfoFocusIndex = 0
			return true
		}
		if s.FocusedOverlayID == "" {
			ids := focusableMarkerIDs(bm)
			if len(ids) == 0 {
				return false
			}
			s.FocusedOverlayID = ids[0]
			nsWrite(w, nsState, c.ID, *s)
			return true
		}
		m := focusedMarker(bm, *s)
		if m == nil {
			s.FocusedOverlayID = ""
			s.InfoOpen = false
			s.InfoFocusIndex = 0
			nsWrite(w, nsState, c.ID, *s)
			return true
		}
		if m.Title != "" {
			s.InfoOpen = true
			s.InfoFocusIndex = 0
		}
		// Persist before the callback so OnPOISelect consumers that
		// mutate map state (PanTo, SetZoom) are not clobbered.
		nsWrite(w, nsState, c.ID, *s)
		if c.OnPOISelect != nil {
			c.OnPOISelect(w, m)
		}
		return true
	case gui.KeyEscape:
		if s.InfoOpen {
			s.InfoOpen = false
			s.InfoFocusIndex = 0
			nsWrite(w, nsState, c.ID, *s)
			return true
		}
		if s.FocusedOverlayID != "" {
			s.FocusedOverlayID = ""
			nsWrite(w, nsState, c.ID, *s)
			return true
		}
		return false
	}
	return false
}

// cycleInfoFocus advances a popup-focus index by step, wrapping across
// the range [0, actionCount] where the trailing slot (== actionCount)
// is the close button. Input index is clamped first so a stale value
// that drifted past the current Action count still produces a sane next
// index. actionCount is clamped to MaxInfoActions so an author-supplied
// Actions slice longer than the draw cap cannot (a) silently wrap via
// int8 truncation or (b) let Tab land on a slot that won't render.
// Negative actionCount collapses to close-only (n=1). Pure —
// Window-free, unit-testable.
func cycleInfoFocus(current int8, actionCount, step int) int8 {
	if actionCount < 0 {
		actionCount = 0
	} else if actionCount > MaxInfoActions {
		actionCount = MaxInfoActions
	}
	n := actionCount + 1 // +1 for the close-button slot
	i := int(current)
	if i < 0 || i >= n {
		i = 0
	}
	i = (i + step) % n
	if i < 0 {
		i += n
	}
	return int8(i)
}

// shiftCenter translates s.Center by (dx, dy) screen-pixels at the
// current fractional zoom.
func shiftCenter(s MapState, dx, dy float64) projection.LatLng {
	p := projection.ProjectF(s.Center, s.Zoom)
	p.X += dx
	p.Y += dy
	return projection.UnprojectF(p, s.Zoom).Clamp()
}
