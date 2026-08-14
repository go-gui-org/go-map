package mapview

import (
	"testing"

	"github.com/go-gui-org/go-gui/gui"
)

// FullWindow must wrap v as the sole content so a consumer passing
// mapview.Map still gets the map at the root of the window.
func TestFullWindow_WrapsChild(t *testing.T) {
	w := &gui.Window{}
	child := gui.Text(gui.TextCfg{Text: "sentinel"})
	kids := FullWindow(w, child).GenerateLayout(w).Children
	if len(kids) != 1 {
		t.Fatalf("content = %d, want 1", len(kids))
	}
	// The generated child must be the passed-in Text view, not an
	// empty placeholder: its shape carries the sentinel string.
	if s := kids[0].Shape; s == nil || s.TC == nil || s.TC.Text != "sentinel" {
		t.Errorf("content[0] does not carry the passed-in child")
	}
}

// Full layout pipeline must produce a root whose single generated
// child is the wrapped view — guards against a refactor that drops
// the child (matches the Legend regression we added earlier).
func TestFullWindow_GenerateLayoutPropagatesChild(t *testing.T) {
	w := &gui.Window{}
	child := gui.Text(gui.TextCfg{Text: "sentinel"})
	root := gui.GenerateViewLayout(FullWindow(w, child), w)
	if got := len(root.Children); got != 1 {
		t.Errorf("Children = %d, want 1", got)
	}
}
