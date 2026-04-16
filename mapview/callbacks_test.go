package mapview

import (
	"math"
	"testing"

	"github.com/mike-ward/go-map/projection"
)

func TestFireDecision_FirstFrameSeedsBaseline(t *testing.T) {
	s := MapState{Center: projection.LatLng{Lat: 1, Lng: 2}, Zoom: 5}
	next, fm, fz := fireDecision(lastFired{}, s)
	if !next.Set || next.State != s {
		t.Errorf("baseline = %+v, want Set=true State=%+v", next, s)
	}
	if fm || fz {
		t.Errorf("first frame must not fire (move=%v zoom=%v)", fm, fz)
	}
}

func TestFireDecision_NoOpWhenStateEqual(t *testing.T) {
	s := MapState{Center: projection.LatLng{Lat: 1, Lng: 2}, Zoom: 5}
	prev := lastFired{State: s, Set: true}
	next, fm, fz := fireDecision(prev, s)
	if next != prev {
		t.Errorf("baseline drifted: %+v -> %+v", prev, next)
	}
	if fm || fz {
		t.Errorf("equal state must not fire (move=%v zoom=%v)", fm, fz)
	}
}

func TestFireDecision_CenterOnlyChange(t *testing.T) {
	prev := lastFired{
		State: MapState{Center: projection.LatLng{Lat: 1, Lng: 2}, Zoom: 5},
		Set:   true,
	}
	now := MapState{Center: projection.LatLng{Lat: 9, Lng: 9}, Zoom: 5}
	next, fm, fz := fireDecision(prev, now)
	if !fm {
		t.Error("center change must fire OnMove")
	}
	if fz {
		t.Error("zoom unchanged must not fire OnZoomChange")
	}
	if next.State != now {
		t.Errorf("baseline = %+v, want %+v", next.State, now)
	}
}

func TestFireDecision_ZoomOnlyChange(t *testing.T) {
	prev := lastFired{
		State: MapState{Center: projection.LatLng{Lat: 1, Lng: 2}, Zoom: 5},
		Set:   true,
	}
	now := MapState{Center: projection.LatLng{Lat: 1, Lng: 2}, Zoom: 7}
	_, fm, fz := fireDecision(prev, now)
	if fm {
		t.Error("center unchanged must not fire OnMove")
	}
	if !fz {
		t.Error("zoom change must fire OnZoomChange")
	}
}

func TestFireDecision_BothChange(t *testing.T) {
	prev := lastFired{
		State: MapState{Center: projection.LatLng{Lat: 0, Lng: 0}, Zoom: 1},
		Set:   true,
	}
	now := MapState{Center: projection.LatLng{Lat: 5, Lng: 5}, Zoom: 9}
	_, fm, fz := fireDecision(prev, now)
	if !fm || !fz {
		t.Errorf("both changed: move=%v zoom=%v", fm, fz)
	}
}

// NaN must flush the accumulator: returning NaN as residual would
// permanently jam the wheel because every future event would compute
// NaN+x = NaN and the consume loops would never fire.
func TestScrollSteps_NaNFlushesToZero(t *testing.T) {
	delta, residual := scrollSteps(float32(math.NaN()))
	if delta != 0 {
		t.Errorf("delta = %d, want 0", delta)
	}
	if math.IsNaN(float64(residual)) {
		t.Errorf("residual = NaN; must be flushed to 0")
	}
	if residual != 0 {
		t.Errorf("residual = %g, want 0", residual)
	}
}

// A single huge ScrollY (or many events between consumes) must not
// run the consume loop millions of times. The accumulator cap binds
// delta to ±maxScrollAccum before the loop starts.
func TestScrollSteps_AccumCapped(t *testing.T) {
	for _, in := range []float32{1e6, 1e9, math.MaxFloat32} {
		delta, _ := scrollSteps(in)
		if int32(delta) > int32(maxScrollAccum) {
			t.Errorf("scrollSteps(%g) delta = %d exceeds cap %g",
				in, delta, maxScrollAccum)
		}
	}
	for _, in := range []float32{-1e6, -1e9, -math.MaxFloat32} {
		delta, _ := scrollSteps(in)
		if int32(delta) < -int32(maxScrollAccum) {
			t.Errorf("scrollSteps(%g) delta = %d below cap -%g",
				in, delta, maxScrollAccum)
		}
	}
}

func TestScrollSteps(t *testing.T) {
	cases := []struct {
		in       float32
		wantD    int32
		wantResi float32
	}{
		{0, 0, 0},
		{0.4, 0, 0.4},   // sub-threshold accumulates
		{1.0, 1, 0},     // exactly one tick
		{2.7, 2, 0.7},   // multi-tick + residual
		{-0.4, 0, -0.4}, // negative sub-threshold
		{-1.5, -1, -0.5},
		{-3.2, -3, -0.2},
	}
	for _, c := range cases {
		gotD, gotR := scrollSteps(c.in)
		if gotD != c.wantD {
			t.Errorf("steps(%g) delta = %d, want %d", c.in, gotD, c.wantD)
		}
		if d := gotR - c.wantResi; d > 1e-6 || d < -1e-6 {
			t.Errorf("steps(%g) residual = %g, want %g", c.in, gotR, c.wantResi)
		}
	}
}
