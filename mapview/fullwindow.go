package mapview

import "github.com/go-gui-org/go-gui/gui"

// FullWindow wraps v in a FixedFixed Column sized to the current
// window, giving a single FillFill child (typically mapview.Map) a
// non-zero root bounds. Without a sized wrapper the view-generator
// root stays 0×0 — renderDrawCanvas bails in rectsOverlap and OnDraw
// never fires. See the root-sizing note in CLAUDE.md.
//
// Equivalent to the pattern every single-widget demo wrote by hand:
//
//	ww, wh := w.WindowSize()
//	return gui.Column(gui.ContainerCfg{
//	    Sizing: gui.FixedFixed,
//	    Width:  float32(ww),
//	    Height: float32(wh),
//	    Content: []gui.View{v},
//	})
//
// Use for a single root widget; a multi-pane window (sidebar + map,
// split views, etc.) still writes its own sized Row / Column so
// layout control stays explicit.
func FullWindow(w *gui.Window, v gui.View) gui.View {
	ww, wh := w.WindowSize()
	return gui.Column(gui.ContainerCfg{
		Sizing:  gui.FixedFixed,
		Width:   float32(ww),
		Height:  float32(wh),
		Padding: gui.Some(gui.Padding{}),
		Content: []gui.View{v},
	})
}
