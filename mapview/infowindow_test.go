package mapview

import (
	"strings"
	"testing"
)

// monospaceMeasure returns a measure function that charges w pixels
// per rune. Sufficient for testing the binary-search truncation
// without a real TextMeasurer.
func monospaceMeasure(pixelsPerRune float32) func(string) float32 {
	return func(s string) float32 {
		// Count runes, not bytes — matches what the real text
		// measurer sees and mirrors truncateToWidth's own rune split.
		return float32(len([]rune(s))) * pixelsPerRune
	}
}

// A string that already fits comes back unchanged — the common case
// on short titles.
func TestTruncateToWidth_UnchangedWhenFits(t *testing.T) {
	got := truncateToWidth("hello", 100, monospaceMeasure(10))
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

// A long string is truncated and suffixed with "…". At 10 px/rune
// with maxW=55, the longest fitting suffix is 4 runes + ellipsis
// (50 px total).
func TestTruncateToWidth_AddsEllipsisWhenTooWide(t *testing.T) {
	got := truncateToWidth("abcdefghij", 55, monospaceMeasure(10))
	want := "abcd…"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Rune boundaries must be preserved — truncation of a multi-byte
// string must never slice through a UTF-8 sequence. Using emoji
// (4 bytes each) so a byte-level truncator would produce garbage.
func TestTruncateToWidth_PreservesRuneBoundaries(t *testing.T) {
	in := "🎯🎯🎯🎯🎯"
	got := truncateToWidth(in, 30, monospaceMeasure(10))
	// Every rune in the output must be a valid emoji; "…" counts.
	for _, r := range got {
		if r == '\ufffd' {
			t.Fatalf("got %q contains replacement rune (invalid UTF-8)", got)
		}
	}
	// Must be shorter than input and end with the ellipsis.
	if !strings.HasSuffix(got, "…") {
		t.Errorf("got %q, want trailing \"…\"", got)
	}
}

// maxW ≤ 0 → empty string. Negative / zero budget means "draw
// nothing" rather than panicking.
func TestTruncateToWidth_NonPositiveBudgetReturnsEmpty(t *testing.T) {
	for _, maxW := range []float32{0, -1, -100} {
		got := truncateToWidth("hello", maxW, monospaceMeasure(10))
		if got != "" {
			t.Errorf("maxW=%v: got %q, want \"\"", maxW, got)
		}
	}
}

// When even the ellipsis alone exceeds the budget, nothing is
// drawable — return empty so the caller knows to skip the text
// entirely rather than paint a bare "…" outside its column.
func TestTruncateToWidth_EllipsisTooWideReturnsEmpty(t *testing.T) {
	// Ellipsis measures 10 px, budget is 5 → can't even fit "…".
	got := truncateToWidth("hello", 5, monospaceMeasure(10))
	if got != "" {
		t.Errorf("got %q, want \"\" (ellipsis alone exceeds budget)", got)
	}
}

// Budget that fits only the ellipsis collapses to just "…".
func TestTruncateToWidth_OnlyEllipsisFits(t *testing.T) {
	// Ellipsis at 10 px, budget 10 → exact fit for "…", nothing
	// else.
	got := truncateToWidth("hello", 10, monospaceMeasure(10))
	if got != "…" {
		t.Errorf("got %q, want %q", got, "…")
	}
}
