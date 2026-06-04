package mapview

import (
	"github.com/go-gui-org/go-gui/gui"
)

// focusRingRadius is the screen-pixel radius of the outline drawn
// around the keyboard-focused marker. Sized to sit just outside the
// 6 px marker disc plus its 2 px white rim so the ring reads as
// distinct from the marker body at standard DPI.
const focusRingRadius float32 = 11

// focusRingWidth is the stroke width of the focus ring.
const focusRingWidth float32 = 2

// focusRingColor is the gold stroke shared by every focus ring
// (marker outline + popup sub-element outline) so all focus states
// read as part of the same visual vocabulary.
var focusRingColor = gui.Hex(0xFFD400)

// infoPopup layout constants. Kept package-local and sized so the
// popup stays legible at the default 11 px body style without
// crowding adjacent markers.
const (
	infoPadX       float32 = 8
	infoPadY       float32 = 6
	infoGap        float32 = 2 // vertical gap between title and body
	infoMaxWidth   float32 = 280
	infoAnchorGap  float32 = 14 // pixels between marker and popup edge
	infoMarginEdge float32 = 4  // min distance from canvas edges
)

// Close-button and action-row layout constants. Close button sits in
// the title row's top-right; action buttons lay out left-to-right in
// a single row below body, centred against popup width.
const (
	infoCloseSize     float32 = 16
	infoCloseGap      float32 = 6 // gap between title end and close box
	infoActionGapY    float32 = 6 // gap between body and action row
	infoActionPadX    float32 = 8
	infoActionPadY    float32 = 3
	infoActionSpacing float32 = 6
)

// infoCloseGlyph is the "×" glyph drawn centred in the close-button
// box. Named so the symbol isn't scattered as a raw literal through
// measurement and draw calls.
const infoCloseGlyph = "×"

// maxInfoTitleBytes / maxInfoBodyBytes cap the UTF-8 byte length of
// text rendered in the popup so a pathological Marker value (bug or
// untrusted import) cannot drive the text measurer or layout math
// into pathological territory. Exceeding strings truncate at a rune
// boundary with a trailing ellipsis.
const (
	maxInfoTitleBytes  = 256
	maxInfoBodyBytes   = 1024
	maxInfoActionBytes = 32
)

// infoTitleStyle / infoBodyStyle share the HUD foreground so the
// popup reads as part of the map chrome. Title sits at 12 px, body
// at 11 px to match the coord readout.
var (
	infoTitleStyle  = gui.TextStyle{Size: 12, Color: hudFG}
	infoBodyStyle   = gui.TextStyle{Size: 11, Color: hudFG}
	infoCloseStyle  = gui.TextStyle{Size: 14, Color: hudFG}
	infoActionStyle = gui.TextStyle{Size: 11, Color: hudFG}
	// infoActionBG is semi-transparent on top of hudBG so the action
	// chip reads as a raised pill without fighting the popup body.
	infoActionBG = gui.Color{R: 255, G: 255, B: 255, A: 36}
)

// infoActionRect records the screen-space rect of one rendered action
// button. Zero-value rect never matches a hit (W or H == 0).
type infoActionRect struct {
	X, Y, W, H float32
}

// infoRectState records the last-rendered popup geometry so
// onMouseDown can consume and dispatch clicks without re-running the
// layout pass. The fixed-size Actions array keeps infoRectState a
// comparable struct (slices aren't comparable) — drawFocus relies on
// equality to skip per-frame state-map writes. MarkerID records which
// overlay owned the popup so action dispatch can look the callback
// back up in the registry at press time. Valid=false means no popup
// is currently drawn.
type infoRectState struct {
	X, Y, W, H                     float32
	CloseX, CloseY, CloseW, CloseH float32
	Actions                        [MaxInfoActions]infoActionRect
	ActionCount                    int
	MarkerID                       string
	Valid                          bool
}

// infoHitKind enumerates popup hit regions. Miss means the point was
// outside every rect.
type infoHitKind int

const (
	infoHitMiss infoHitKind = iota
	infoHitBody
	infoHitClose
	infoHitAction
)

// infoHitResult is the lookup outcome. Index is valid only when
// Kind == infoHitAction.
type infoHitResult struct {
	Kind  infoHitKind
	Index int
}

// hit classifies (px, py) against the stored popup rects in priority
// order: close button first (nested inside body), then each action,
// then the body fill. Invalid rects never match.
func (r infoRectState) hit(px, py float32) infoHitResult {
	if !r.Valid {
		return infoHitResult{Kind: infoHitMiss}
	}
	if r.CloseW > 0 && r.CloseH > 0 &&
		px >= r.CloseX && px < r.CloseX+r.CloseW &&
		py >= r.CloseY && py < r.CloseY+r.CloseH {
		return infoHitResult{Kind: infoHitClose}
	}
	// Clamp the loop bound to the fixed-array capacity so a corrupt
	// state-registry entry with ActionCount > MaxInfoActions (bug or
	// stale read during struct evolution) cannot index past r.Actions.
	n := r.ActionCount
	if n > MaxInfoActions {
		n = MaxInfoActions
	}
	for i := 0; i < n; i++ {
		a := r.Actions[i]
		if a.W <= 0 || a.H <= 0 {
			continue
		}
		if px >= a.X && px < a.X+a.W && py >= a.Y && py < a.Y+a.H {
			return infoHitResult{Kind: infoHitAction, Index: i}
		}
	}
	if px >= r.X && px < r.X+r.W && py >= r.Y && py < r.Y+r.H {
		return infoHitResult{Kind: infoHitBody}
	}
	return infoHitResult{Kind: infoHitMiss}
}

// drawFocus renders the focus ring around the keyboard-focused marker
// and, when s.InfoOpen, the InfoWindow popup anchored to it. The
// popup rects are stashed in the state registry so input handlers can
// consume and dispatch clicks that land on the popup. Callers resolve
// the focused marker once per frame (shared with stateForA11Y) and
// pass it in; nil means viewport mode. The rect write is guarded
// against equality so a static popup doesn't bump the state map on
// every frame.
func drawFocus(w *gui.Window, id string, dc *gui.DrawContext, vp viewport, m *Marker, s MapState) {
	if m == nil {
		clearInfoRect(w, id)
		return
	}
	mx, my := vp.LatLngToScreen(m.Pos)
	// Skip when the projection produced non-finite screen coords (bad
	// author Pos slipped past projection.Clamp, NaN zoom) — drawing
	// primitives would propagate NaN geometry into the tessellation
	// buffer and corrupt every subsequent triangle in the batch.
	if !isFiniteF32(mx) || !isFiniteF32(my) {
		clearInfoRect(w, id)
		return
	}
	dc.Circle(mx, my, focusRingRadius, focusRingColor, focusRingWidth)
	if !s.InfoOpen || m.Title == "" {
		clearInfoRect(w, id)
		return
	}
	next := drawInfoWindow(dc, mx, my, m, s.InfoFocusIndex)
	if !next.Valid {
		clearInfoRect(w, id)
		return
	}
	next.MarkerID = m.MarkerID
	if nsRead[infoRectState](w, nsInfoRect, id) != next {
		nsWrite(w, nsInfoRect, id, next)
	}
}

// clearInfoRect marks the popup rect as absent. Skips the write when
// the slot is already empty so a map without a focused marker does
// not dirty the state map every frame.
func clearInfoRect(w *gui.Window, id string) {
	if !nsRead[infoRectState](w, nsInfoRect, id).Valid {
		return
	}
	nsWrite(w, nsInfoRect, id, infoRectState{})
}

// truncateToWidth returns s, possibly with its trailing runes
// replaced by a single "…", so the measured width is at most maxW.
// Pure-function core — takes the measure fn directly instead of a
// *DrawContext so tests do not need a real TextMeasurer.
//
// Binary-searches over rune count so mid-rune truncation is
// impossible (multi-byte UTF-8, emoji, combining marks stay whole).
// Degenerate inputs:
//   - maxW ≤ 0 → "" (no budget to draw anything).
//   - s already fits → returned unchanged.
//   - "…" alone doesn't fit → "" (budget too tight for even an
//     ellipsis).
func truncateToWidth(s string, maxW float32, measure func(string) float32) string {
	if maxW <= 0 {
		return ""
	}
	if measure(s) <= maxW {
		return s
	}
	const ellipsis = "…"
	if measure(ellipsis) > maxW {
		return ""
	}
	runes := []rune(s)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if measure(string(runes[:mid])+ellipsis) <= maxW {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		return ellipsis
	}
	return string(runes[:lo]) + ellipsis
}

// isFiniteF32 reports whether v is a real finite number (not NaN and
// not ±Inf). Drawing paths use this before feeding coords to the
// DrawContext so a single bad value cannot poison the triangle batch.
// Delegates to isFinite so every "reject non-finite" guard in the
// package reads from the same source.
func isFiniteF32(v float32) bool {
	return isFinite(float64(v))
}

func mousePositionFinite(e *gui.Event) bool {
	return isFiniteF32(e.MouseX) && isFiniteF32(e.MouseY)
}

// drawInfoWindow paints the popup box anchored to (mx, my) — the
// marker's screen position. Returns the final geometry so drawFocus
// can persist it for input hit-testing. The popup sits above the
// marker by default; when that would clip the canvas top it flips
// below. Horizontal placement centers on the marker and clamps to
// keep the popup fully on-screen. Non-finite / zero canvas size short-
// circuits to a zero-Valid result so the caller clears any stale rect.
func drawInfoWindow(dc *gui.DrawContext, mx, my float32, m *Marker, focusIdx int8) infoRectState {
	if !isFiniteF32(dc.Width) || !isFiniteF32(dc.Height) ||
		dc.Width <= 0 || dc.Height <= 0 {
		return infoRectState{}
	}
	title := truncateUTF8(m.Title, maxInfoTitleBytes)
	body := m.Body
	if body == "" {
		body = m.Label
	}
	body = truncateUTF8(body, maxInfoBodyBytes)

	// Clamp the title to the pixel budget the title row actually has
	// (popup cap minus close-button column). The byte cap upstream
	// bounds memory; this second step bounds rendered width so a
	// long label cannot overflow into the close box at the current
	// font. No-op for tests without a text measurer — TextWidth
	// returns 0 and truncateToWidth then treats the title as "fits".
	titleBudget := infoMaxWidth - infoCloseGap - infoCloseSize
	title = truncateToWidth(title, titleBudget, func(s string) float32 {
		return dc.TextWidth(s, infoTitleStyle)
	})

	// Measure content widths. Title row must budget a close-button
	// column so the X never overlaps glyphs.
	titleW := dc.TextWidth(title, infoTitleStyle)
	bodyW := dc.TextWidth(body, infoBodyStyle)
	titleRowW := titleW + infoCloseGap + infoCloseSize

	// Action row: measure buttons up to cap, track individual widths so
	// the draw pass doesn't re-measure. Entries past MaxInfoActions or
	// with empty labels are skipped.
	var actionWidths [MaxInfoActions]float32
	var actionLabels [MaxInfoActions]string
	actionCount := 0
	actionRowW := float32(0)
	actionH := float32(0)
	for _, a := range m.Actions {
		if actionCount >= MaxInfoActions {
			break
		}
		label := truncateUTF8(a.Label, maxInfoActionBytes)
		if label == "" {
			continue
		}
		lw := dc.TextWidth(label, infoActionStyle) + infoActionPadX*2
		// A broken TextWidth (NaN / ±Inf / negative) would poison
		// actionRowW and every downstream actions[i].X/W, which the
		// focus-ring RoundedRect and the chip FilledRoundedRect would
		// then feed to the tessellator. Skip the entry — same posture
		// as drawInfoWindow's title/body width guard.
		if !isFiniteF32(lw) || lw <= 0 {
			continue
		}
		actionLabels[actionCount] = label
		actionWidths[actionCount] = lw
		if actionCount > 0 {
			actionRowW += infoActionSpacing
		}
		actionRowW += lw
		actionCount++
	}
	if actionCount > 0 {
		actionH = dc.FontHeight(infoActionStyle) + infoActionPadY*2
	}

	contentW := titleRowW
	if bodyW > contentW {
		contentW = bodyW
	}
	if actionRowW > contentW {
		contentW = actionRowW
	}
	if contentW > infoMaxWidth {
		contentW = infoMaxWidth
	}

	titleH := dc.FontHeight(infoTitleStyle)
	bodyH := dc.FontHeight(infoBodyStyle)
	w := contentW + infoPadX*2
	h := titleH + infoPadY*2
	if body != "" {
		h += bodyH + infoGap
	}
	if actionCount > 0 {
		h += actionH + infoActionGapY
	}
	// A broken font metric (NaN / ±Inf TextWidth or FontHeight) would
	// propagate into w/h here and then into every tessellation vertex
	// the popup emits — the same failure mode slice 3 guarded against
	// for focus-ring screen coords. Bail to a zero-Valid rect so the
	// caller clears any stale state instead of painting NaN geometry.
	if !isFiniteF32(w) || !isFiniteF32(h) || w <= 0 || h <= 0 {
		return infoRectState{}
	}

	x := mx - w/2
	y := my - infoAnchorGap - h
	// Clamp horizontally within canvas.
	if x < infoMarginEdge {
		x = infoMarginEdge
	}
	if x+w > dc.Width-infoMarginEdge {
		x = dc.Width - infoMarginEdge - w
	}
	// Right-edge clamp can push x negative when the popup is wider
	// than the canvas (narrow windows). Final left-edge pin keeps at
	// least the popup's left side on-screen.
	if x < 0 {
		x = 0
	}
	// Flip below the marker when the popup would clip the canvas top.
	if y < infoMarginEdge {
		y = my + infoAnchorGap
	}
	// Final bottom-edge clamp (handles the flipped-below case on a
	// canvas too short for either anchor).
	if y+h > dc.Height-infoMarginEdge {
		y = dc.Height - infoMarginEdge - h
	}
	if y < 0 {
		y = 0
	}

	dc.FilledRoundedRect(x, y, w, h, 4, hudBG)
	dc.Text(x+infoPadX, y+infoPadY, title, infoTitleStyle)
	if body != "" {
		dc.Text(x+infoPadX, y+infoPadY+titleH+infoGap, body, infoBodyStyle)
	}

	// Close button: flush top-right within padX. Background matches
	// action chip so it reads as a tappable region; "×" glyph sits
	// centred inside. The close row shares the title's vertical band,
	// so centre the box on the title's vertical midline.
	closeX := x + w - infoPadX - infoCloseSize
	closeY := y + infoPadY + (titleH-infoCloseSize)/2
	if closeY < y+2 {
		closeY = y + 2
	}
	dc.FilledRoundedRect(closeX, closeY, infoCloseSize, infoCloseSize, 3, infoActionBG)
	closeGlyphW := dc.TextWidth(infoCloseGlyph, infoCloseStyle)
	closeGlyphH := dc.FontHeight(infoCloseStyle)
	dc.Text(
		closeX+(infoCloseSize-closeGlyphW)/2,
		closeY+(infoCloseSize-closeGlyphH)/2,
		infoCloseGlyph, infoCloseStyle,
	)

	// Action row: centered horizontally within the popup, anchored to
	// the bottom padding. Record each rect for hit-testing. actionH
	// already embeds FontHeight + 2*padY, so deriving glyphH from it
	// (instead of re-calling FontHeight per button) saves N measurer
	// calls per popup frame.
	var actions [MaxInfoActions]infoActionRect
	if actionCount > 0 {
		rowY := y + h - infoPadY - actionH
		// Centre the action row when it fits. When actionRowW exceeds
		// the popup's content width (the 4-wide edge case flagged in
		// plan §4a), (w-actionRowW)/2 goes negative and the left-most
		// chip would leak past the popup's left edge. Clamp to infoPadX
		// so overflow spills right (still wrong visually, but stays
		// inside the window frame and keeps hit-dispatch consistent).
		rowX := x + (w-actionRowW)/2
		if rowX < x+infoPadX {
			rowX = x + infoPadX
		}
		cx := rowX
		glyphH := actionH - infoActionPadY*2
		for i := 0; i < actionCount; i++ {
			bw := actionWidths[i]
			actions[i] = infoActionRect{X: cx, Y: rowY, W: bw, H: actionH}
			dc.FilledRoundedRect(cx, rowY, bw, actionH, 3, infoActionBG)
			dc.Text(
				cx+infoActionPadX,
				rowY+(actionH-glyphH)/2,
				actionLabels[i], infoActionStyle,
			)
			cx += bw + infoActionSpacing
		}
	}

	// Focus ring drawn last so the outline sits on top of chip fills.
	// Out-of-range indices paint nothing — a stale focus index drops
	// silently until the next open reseeds it.
	idx := int(focusIdx)
	switch {
	case idx >= 0 && idx < actionCount:
		a := actions[idx]
		dc.RoundedRect(a.X, a.Y, a.W, a.H, 3,
			focusRingColor, focusRingWidth)
	case idx == actionCount:
		dc.RoundedRect(closeX, closeY, infoCloseSize, infoCloseSize, 3,
			focusRingColor, focusRingWidth)
	}

	return infoRectState{
		X: x, Y: y, W: w, H: h,
		CloseX: closeX, CloseY: closeY,
		CloseW: infoCloseSize, CloseH: infoCloseSize,
		Actions:     actions,
		ActionCount: actionCount,
		Valid:       true,
	}
}

// truncateUTF8 returns s unchanged when len(s) <= limit; otherwise it
// trims to the largest rune boundary at or below limit and appends
// "…" so the result remains a valid UTF-8 string. limit < 0 is
// treated as 0; s shorter than one rune cannot be truncated further.
func truncateUTF8(s string, limit int) string {
	if limit < 0 {
		limit = 0
	}
	if len(s) <= limit {
		return s
	}
	end := limit
	// UTF-8 continuation bytes are 0b10xxxxxx; roll back so we never
	// split a multibyte rune.
	for end > 0 && (s[end]&0xC0) == 0x80 {
		end--
	}
	return s[:end] + "…"
}

// handlePopupClick consumes a mouse-down that landed on the InfoWindow
// popup. Close-button and action-button hits dispatch on press
// (matching the Home button) so no drag-tracking is started; the
// state write to close the popup fires *before* any action callback so
// an OnClick that reads Snapshot sees InfoOpen=false. Returns true when
// the event was consumed (body hits, close, action); false means no
// popup is drawn or the press was outside the popup rect — caller
// continues with its normal handling.
func handlePopupClick(w *gui.Window, id string, e *gui.Event) bool {
	rect := nsRead[infoRectState](w, nsInfoRect, id)
	h := rect.hit(e.MouseX, e.MouseY)
	switch h.Kind {
	case infoHitMiss:
		return false
	case infoHitBody:
		e.IsHandled = true
		return true
	case infoHitClose:
		closeInfoPopup(w, id)
		e.IsHandled = true
		return true
	case infoHitAction:
		dispatchInfoAction(w, id, markerByID(w, id, rect.MarkerID), h.Index)
		e.IsHandled = true
		return true
	}
	return false
}

// dispatchInfoAction closes the popup and, when idx is in range, fires
// the action callback. Invariant shared with the keyboard Enter path:
// registry state is persisted with InfoOpen=false *before* the callback
// runs so any Snapshot read inside the callback sees the dismissed
// state. Bounds-guarded against both MaxInfoActions and the live
// Actions length so a stale index from a shrunken Actions slice cannot
// OOB.
func dispatchInfoAction(w *gui.Window, id string, m *Marker, idx int) {
	closeInfoPopup(w, id)
	if m == nil || idx < 0 || idx >= MaxInfoActions || idx >= len(m.Actions) {
		return
	}
	if cb := m.Actions[idx].OnClick; cb != nil {
		cb(w)
	}
}

// closeInfoPopup flips InfoOpen off on the map state and resets the
// popup focus index so the next open lands on the first sub-element.
// No-op when the popup is already closed so a second close press on a
// race does not dirty the state map.
func closeInfoPopup(w *gui.Window, id string) {
	s := nsRead[MapState](w, nsState, id)
	if !s.InfoOpen && s.InfoFocusIndex == 0 {
		return
	}
	s.InfoOpen = false
	s.InfoFocusIndex = 0
	nsWrite(w, nsState, id, s)
}

// markerByID fetches the Marker overlay with the given overlay-ID,
// returning nil when absent or when the overlay is not a Marker. Used
// by action dispatch to re-resolve the callback at press time — the
// overlay map may have changed between the frame that rendered the
// popup and the frame that delivered the click.
func markerByID(w *gui.Window, id, markerID string) *Marker {
	if markerID == "" {
		return nil
	}
	o, ok := readOverlays(w, id).Get(markerID)
	if !ok {
		return nil
	}
	m, ok := o.(*Marker)
	if !ok {
		return nil
	}
	return m
}
