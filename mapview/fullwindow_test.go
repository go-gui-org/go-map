package mapview

import (
	"testing"

	"github.com/mike-ward/go-gui/gui"
)

// FullWindow must wrap v as the sole content so a consumer passing
// mapview.Map still gets the map at the root of the window.
func TestFullWindow_WrapsChild(t *testing.T) {
	w := &gui.Window{}
	child := gui.Text(gui.TextCfg{Text: "sentinel"})
	got := FullWindow(w, child)
	content := got.Content()
	if len(content) != 1 {
		t.Fatalf("content = %d, want 1", len(content))
	}
	if content[0] != child {
		t.Errorf("content[0] = %v, want the passed-in child", content[0])
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
