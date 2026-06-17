package mapview

import "github.com/go-gui-org/go-gui/gui"

// layoutRecursive mirrors the now-deprecated gui.GenerateViewLayout:
// generates v's own layout then recurses into Content() children.
// Needed because our thin wrapper Views (gallery, legend) return nil
// from Content() and build the full tree inside GenerateLayout.
func layoutRecursive(v gui.View, w *gui.Window) gui.Layout {
	l := v.GenerateLayout(w)
	for _, child := range v.Content() {
		if child != nil {
			l.Children = append(l.Children, layoutRecursive(child, w))
		}
	}
	return l
}
