package mapview

import (
	"time"

	"github.com/mike-ward/go-gui/gui"
)

// a11yDebounce is the idle window before a new map state is
// announced. Plan §Perceivable: screen readers must not be spammed
// with every intermediate pan/zoom frame. 300 ms matches the spec
// and is long enough to suppress a sustained drag while short enough
// that a single deliberate pan lands within one beat.
const a11yDebounce = 300 * time.Millisecond

// a11yDebounceState tracks the announcement machinery per map ID.
// Announced is the string currently exposed via A11YDescription.
// Pending is the desired next announcement; At is when Pending first
// diverged from Announced. Timer holds the single in-flight wake for
// the pending target so a new target can Stop() the prior scheduled
// refresh before installing its own — keeps exactly one timer per
// map instead of one per frame, and releases the captured Window
// reference as soon as the target is superseded.
type a11yDebounceState struct {
	Announced string
	Pending   string
	At        time.Time
	Timer     *time.Timer
}

// a11yTimeNow and a11yScheduleWake are indirection points for tests.
// The defaults route through real time.Now and a time.AfterFunc that
// queues a full layout refresh. RequestRedraw is not enough — it
// triggers only UpdateRenderOnly, which rebuilds renderers from the
// cached layout tree without re-invoking the view generator, so
// debouncedA11Y would never run to promote the pending string. The
// refresh itself must happen on the UI goroutine, so the timer hands
// off through QueueCommand. Tests swap both to drive the clock
// synchronously and capture scheduled wakes.
var (
	a11yTimeNow      = time.Now
	a11yScheduleWake = func(w *gui.Window, d time.Duration) *time.Timer {
		if w == nil {
			return nil
		}
		return time.AfterFunc(d, func() {
			w.QueueCommand(func(ww *gui.Window) { ww.UpdateWindow() })
		})
	}
)

// debouncedA11Y returns the A11YDescription string the widget should
// expose this frame, holding back rapidly-changing announcements so
// screen readers hear only settled state.
//
// The go-gui frame loop is event-driven — with no input there is no
// frame, so the debounce relies on a timer to wake the window once
// the idle window elapses. Each new target cancels the prior timer
// and installs its own, so a sustained drag holds a single wake at a
// time rather than one per frame.
func debouncedA11Y(w *gui.Window, id, current string) string {
	a := nsRead[a11yDebounceState](w, nsA11y, id)
	if current == "" {
		// Never promote an empty string into Announced. Storing ""
		// would re-match the first-call branch below on every
		// subsequent frame, turning a degenerate stateForA11Y into
		// an unbounded nsWrite loop. Hold whatever prior announcement
		// exists until a real string arrives.
		return a.Announced
	}
	now := a11yTimeNow()
	if a.Announced == "" && a.Pending == "" && a.Timer == nil {
		// First announcement. Skip the debounce — a screen reader
		// waking to an empty description for 300 ms on a fresh focus
		// is strictly worse than announcing the seed state.
		settleA11Y(w, id, current)
		return current
	}
	if current == a.Announced {
		if a.Pending != "" || a.Timer != nil {
			stopTimer(a.Timer)
			settleA11Y(w, id, a.Announced)
		}
		return a.Announced
	}
	if current != a.Pending {
		stopTimer(a.Timer)
		a = a11yDebounceState{
			Announced: a.Announced,
			Pending:   current,
			At:        now,
			Timer:     a11yScheduleWake(w, a11yDebounce),
		}
		nsWrite(w, nsA11y, id, a)
	}
	if now.Sub(a.At) >= a11yDebounce {
		stopTimer(a.Timer)
		settleA11Y(w, id, current)
		return current
	}
	return a.Announced
}

// settleA11Y writes the debounce slot down to just an Announced
// string, clearing Pending / At / Timer. Used by every branch that
// reaches a resting state: first-call promote, revert-to-announced,
// and debounce-elapsed promote.
func settleA11Y(w *gui.Window, id, announced string) {
	nsWrite(w, nsA11y, id, a11yDebounceState{Announced: announced})
}

// stopTimer is a nil-safe Stop. Writing the guard inline at three
// call sites clutters debouncedA11Y; keeping it local avoids forcing
// callers to think about the nil branch.
func stopTimer(t *time.Timer) {
	if t != nil {
		t.Stop()
	}
}
