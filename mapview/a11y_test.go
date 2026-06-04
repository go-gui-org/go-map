package mapview

import (
	"sync"
	"testing"
	"time"

	"github.com/go-gui-org/go-gui/gui"
)

// a11yTestClock drives debouncedA11Y's time source so tests can
// advance wall-clock time deterministically without sleeping.
type a11yTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func newA11yTestClock() *a11yTestClock {
	return &a11yTestClock{now: time.Unix(1_700_000_000, 0)}
}

func (c *a11yTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *a11yTestClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// withA11YStubs installs a stub clock and a no-op scheduler for the
// duration of a subtest, restoring the real implementations on exit.
// Real time.AfterFunc would spawn goroutines that outlive the test.
func withA11YStubs(t *testing.T) (*a11yTestClock, *int) {
	t.Helper()
	origNow, origSched := a11yTimeNow, a11yScheduleWake
	clock := newA11yTestClock()
	wakeCalls := 0
	a11yTimeNow = clock.Now
	a11yScheduleWake = func(_ *gui.Window, _ time.Duration) *time.Timer {
		wakeCalls++
		return nil
	}
	t.Cleanup(func() {
		a11yTimeNow, a11yScheduleWake = origNow, origSched
	})
	return clock, &wakeCalls
}

// First call on a fresh id promotes immediately so the seed state
// reaches assistive tech without a 300 ms black hole.
func TestDebouncedA11Y_FirstCallPromotes(t *testing.T) {
	_, wakes := withA11YStubs(t)
	w := &gui.Window{}
	got := debouncedA11Y(w, "m", "zoom 12")
	if got != "zoom 12" {
		t.Errorf("first call = %q, want %q", got, "zoom 12")
	}
	if *wakes != 0 {
		t.Errorf("first call scheduled %d wakes, want 0", *wakes)
	}
}

// A stable sequence of identical strings must never re-write or
// re-schedule after the initial promote.
func TestDebouncedA11Y_StableRepeat(t *testing.T) {
	_, wakes := withA11YStubs(t)
	w := &gui.Window{}
	debouncedA11Y(w, "m", "stable")
	*wakes = 0
	for range 5 {
		if got := debouncedA11Y(w, "m", "stable"); got != "stable" {
			t.Errorf("repeat = %q, want %q", got, "stable")
		}
	}
	if *wakes != 0 {
		t.Errorf("stable sequence scheduled %d wakes, want 0", *wakes)
	}
}

// A change within the idle window must hold the old announcement and
// schedule a wake. Only after the clock advances past a11yDebounce
// does the new target promote.
func TestDebouncedA11Y_HoldsThenPromotes(t *testing.T) {
	clock, wakes := withA11YStubs(t)
	w := &gui.Window{}
	debouncedA11Y(w, "m", "A")
	*wakes = 0

	if got := debouncedA11Y(w, "m", "B"); got != "A" {
		t.Errorf("mid-debounce = %q, want A", got)
	}
	if *wakes != 1 {
		t.Errorf("mid-debounce scheduled %d wakes, want 1", *wakes)
	}

	clock.Advance(a11yDebounce / 2)
	if got := debouncedA11Y(w, "m", "B"); got != "A" {
		t.Errorf("half-window = %q, want A", got)
	}

	clock.Advance(a11yDebounce)
	if got := debouncedA11Y(w, "m", "B"); got != "B" {
		t.Errorf("post-window = %q, want B", got)
	}
}

// A sustained drag repeatedly presents the same pending target frame
// after frame. Only the first frame must schedule a wake — subsequent
// frames against the same pending must not stack timers. Earlier
// design scheduled per-frame, producing ~18 concurrent timers during
// a 300 ms drag; the invariant here pins the single-timer rewrite.
func TestDebouncedA11Y_SingleTimerPerTarget(t *testing.T) {
	clock, wakes := withA11YStubs(t)
	w := &gui.Window{}
	debouncedA11Y(w, "m", "A")
	*wakes = 0
	for range 30 {
		clock.Advance(a11yDebounce / 60)
		if got := debouncedA11Y(w, "m", "B"); got != "A" {
			t.Fatalf("mid-drag = %q, want A", got)
		}
	}
	if *wakes != 1 {
		t.Errorf("sustained drag scheduled %d wakes, want 1", *wakes)
	}
}

// Promotion triggers when the clock lands exactly on a11yDebounce.
// The predicate uses >= so a clock tick exactly at the boundary must
// flip. Earlier drafts with > would leave the announcement stuck for
// one extra frame — cheap to drift, hard to notice.
func TestDebouncedA11Y_PromotesAtExactBoundary(t *testing.T) {
	clock, _ := withA11YStubs(t)
	w := &gui.Window{}
	debouncedA11Y(w, "m", "A")
	debouncedA11Y(w, "m", "B")
	clock.Advance(a11yDebounce - 1)
	if got := debouncedA11Y(w, "m", "B"); got != "A" {
		t.Fatalf("just-before = %q, want A", got)
	}
	clock.Advance(1)
	if got := debouncedA11Y(w, "m", "B"); got != "B" {
		t.Errorf("at boundary = %q, want B", got)
	}
}

// Two map IDs in one Window must debounce independently. The nsA11y
// StateMap is keyed by id, so structural isolation is expected; this
// pins it so a future refactor that collapses the key doesn't go
// unnoticed.
func TestDebouncedA11Y_PerID(t *testing.T) {
	clock, _ := withA11YStubs(t)
	w := &gui.Window{}
	debouncedA11Y(w, "a", "Aseed")
	debouncedA11Y(w, "b", "Bseed")
	debouncedA11Y(w, "a", "Anext")
	clock.Advance(a11yDebounce)
	if got := debouncedA11Y(w, "a", "Anext"); got != "Anext" {
		t.Errorf("a promoted = %q, want Anext", got)
	}
	if got := debouncedA11Y(w, "b", "Bseed"); got != "Bseed" {
		t.Errorf("b stable = %q, want Bseed", got)
	}
}

// Resetting the target before promotion restarts the timer so a
// sustained drag never surfaces intermediate strings.
func TestDebouncedA11Y_ResetsOnNewTarget(t *testing.T) {
	clock, _ := withA11YStubs(t)
	w := &gui.Window{}
	debouncedA11Y(w, "m", "A")

	debouncedA11Y(w, "m", "B")
	clock.Advance(a11yDebounce - 10*time.Millisecond)
	// Pending shifts to C just before the B timer would have fired —
	// the B promotion should be cancelled and only C can surface, and
	// only after another full idle window.
	if got := debouncedA11Y(w, "m", "C"); got != "A" {
		t.Errorf("new target mid-window = %q, want A", got)
	}
	clock.Advance(a11yDebounce - 1)
	if got := debouncedA11Y(w, "m", "C"); got != "A" {
		t.Errorf("just under = %q, want A", got)
	}
	clock.Advance(1)
	if got := debouncedA11Y(w, "m", "C"); got != "C" {
		t.Errorf("promoted = %q, want C", got)
	}
}

// Reverting to the announced string mid-debounce clears the pending
// target so the next wake doesn't fire on stale state.
func TestDebouncedA11Y_RevertClearsPending(t *testing.T) {
	clock, _ := withA11YStubs(t)
	w := &gui.Window{}
	debouncedA11Y(w, "m", "A")
	debouncedA11Y(w, "m", "B")
	// Revert before debounce elapses.
	if got := debouncedA11Y(w, "m", "A"); got != "A" {
		t.Errorf("revert = %q, want A", got)
	}
	clock.Advance(2 * a11yDebounce)
	// Another "A" must not resurrect "B".
	if got := debouncedA11Y(w, "m", "A"); got != "A" {
		t.Errorf("post-revert stable = %q, want A", got)
	}
	persisted := nsRead[a11yDebounceState](w, nsA11y, "m")
	if persisted.Pending != "" {
		t.Errorf("pending lingered after revert: %q", persisted.Pending)
	}
}

// An empty current must never be promoted into Announced.
// Otherwise every subsequent frame matches the first-call branch
// and re-writes nsA11y, defeating the whole debounce.
func TestDebouncedA11Y_EmptyCurrentHeld(t *testing.T) {
	_, wakes := withA11YStubs(t)
	w := &gui.Window{}
	debouncedA11Y(w, "m", "settled")
	*wakes = 0
	for range 5 {
		if got := debouncedA11Y(w, "m", ""); got != "settled" {
			t.Fatalf("empty call = %q, want settled", got)
		}
	}
	if *wakes != 0 {
		t.Errorf("empty calls scheduled %d wakes, want 0", *wakes)
	}
	persisted := nsRead[a11yDebounceState](w, nsA11y, "m")
	if persisted.Announced != "settled" {
		t.Errorf("announced = %q, want settled", persisted.Announced)
	}
}

// nsA11y writes must not bump the DrawCanvas version — the
// announcement machinery is invisible to OnDraw and cannot be
// allowed to invalidate the tessellation cache every frame a
// debounce is in flight.
func TestDebouncedA11Y_DoesNotBumpVersion(t *testing.T) {
	_, _ = withA11YStubs(t)
	w := &gui.Window{}
	debouncedA11Y(w, "m", "A")
	if v := readVersion(w, "m"); v != 0 {
		t.Errorf("first-call bumped version to %d", v)
	}
	debouncedA11Y(w, "m", "B")
	if v := readVersion(w, "m"); v != 0 {
		t.Errorf("pending write bumped version to %d", v)
	}
}
