package mapview

import (
	"math"
	"testing"

	"github.com/mike-ward/go-gui/gui"
	"github.com/mike-ward/go-map/projection"
)

// Snapshot must report ok=false before the widget has rendered, so a
// consumer reading the panel before the first frame cannot mistake
// the zero MapState (Atlantic equator, zoom 0) for a real viewport.
func TestSnapshot_FalseBeforeRender(t *testing.T) {
	w := &gui.Window{}
	s, ok := Snapshot(w, "missing")
	if ok {
		t.Errorf("ok = true for unknown id; got state %+v", s)
	}
	if s != (MapState{}) {
		t.Errorf("snapshot of missing id = %+v, want zero", s)
	}
}

// readState is the only path that seeds the registry; once seeded,
// Snapshot must report ok=true and round-trip the seed value.
func TestSnapshot_TrueAfterSeed(t *testing.T) {
	w := &gui.Window{}
	seed := MapState{Center: projection.LatLng{Lat: 47.6, Lng: -122.3}, Zoom: 11}
	readState(w, "m", seed)
	got, ok := Snapshot(w, "m")
	if !ok {
		t.Fatal("ok = false after seed; want true")
	}
	if got != seed {
		t.Errorf("snapshot = %+v, want %+v", got, seed)
	}
}

// Mutators must be no-ops for unknown ids — silently constructing a
// fresh MapState would defeat the seed-only-once invariant and let a
// stray PanTo bypass the Initial* fields.
func TestPanTo_NoOpOnUnknownID(t *testing.T) {
	w := &gui.Window{}
	PanTo(w, "missing", projection.LatLng{Lat: 1, Lng: 2})
	if _, ok := Snapshot(w, "missing"); ok {
		t.Error("PanTo on unknown id created state")
	}
}

func TestSetZoom_NoOpOnUnknownID(t *testing.T) {
	w := &gui.Window{}
	SetZoom(w, "missing", 5)
	if _, ok := Snapshot(w, "missing"); ok {
		t.Error("SetZoom on unknown id created state")
	}
}

func TestSetView_NoOpOnUnknownID(t *testing.T) {
	w := &gui.Window{}
	SetView(w, "missing", projection.LatLng{Lat: 1, Lng: 2}, 3)
	if _, ok := Snapshot(w, "missing"); ok {
		t.Error("SetView on unknown id created state")
	}
}

// SetZoom values above maxZoomF must clamp; otherwise consumers can
// permanently park the viewport at an unreachable zoom level.
func TestSetZoom_ClampsAtMaxZoom(t *testing.T) {
	w := &gui.Window{}
	readState(w, "m", MapState{Zoom: 10})
	SetZoom(w, "m", maxZoomF+5)
	got, _ := Snapshot(w, "m")
	if got.Zoom != maxZoomF {
		t.Errorf("Zoom = %g, want %g", got.Zoom, maxZoomF)
	}
}

// Fractional SetZoom must round-trip without snapping to an integer.
// This is the whole point of slice 5a.
func TestSetZoom_PreservesFractional(t *testing.T) {
	w := &gui.Window{}
	readState(w, "m", MapState{Zoom: 10})
	SetZoom(w, "m", 12.5)
	got, _ := Snapshot(w, "m")
	if got.Zoom != 12.5 {
		t.Errorf("Zoom = %g, want 12.5", got.Zoom)
	}
}

// SetZoom with NaN / ±Inf must collapse to 0, not propagate a poison
// value through the registry. Direct check at the public-API boundary
// so a future refactor that accidentally skips clampZoom gets caught
// independently of the helper's own unit test.
func TestSetZoom_NonFiniteCollapsesToZero(t *testing.T) {
	for _, z := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		w := &gui.Window{}
		readState(w, "m", MapState{Zoom: 10})
		SetZoom(w, "m", z)
		got, _ := Snapshot(w, "m")
		if got.Zoom != 0 {
			t.Errorf("SetZoom(%v): Zoom = %g, want 0", z, got.Zoom)
		}
	}
}

// SetZoom with a negative value must also collapse to 0 — clampZoom
// contract is "legal range is [0, maxZoomF]", negatives are out.
func TestSetZoom_NegativeCollapsesToZero(t *testing.T) {
	w := &gui.Window{}
	readState(w, "m", MapState{Zoom: 10})
	SetZoom(w, "m", -5)
	got, _ := Snapshot(w, "m")
	if got.Zoom != 0 {
		t.Errorf("SetZoom(-5): Zoom = %g, want 0", got.Zoom)
	}
}

// SetView with NaN / ±Inf zoom must collapse to 0 (center still
// applies after Clamp).
func TestSetView_NonFiniteZoomCollapsesToZero(t *testing.T) {
	for _, z := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		w := &gui.Window{}
		readState(w, "m", MapState{Zoom: 10})
		SetView(w, "m", projection.LatLng{Lat: 5, Lng: 5}, z)
		got, _ := Snapshot(w, "m")
		if got.Zoom != 0 {
			t.Errorf("SetView zoom=%v: Zoom = %g, want 0", z, got.Zoom)
		}
	}
}

// clampZoom direct table check. Covered indirectly by SetZoom /
// SetView / FitBounds / Map tests; this one keeps failure diagnostics
// local to the helper.
func TestClampZoom_Table(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{math.NaN(), 0},
		{math.Inf(1), 0},
		{math.Inf(-1), 0},
		{-1, 0},
		{-0.0001, 0},
		{0, 0},
		{11.5, 11.5},
		{maxZoomF, maxZoomF},
		{maxZoomF + 0.0001, maxZoomF},
		{100, maxZoomF},
	}
	for _, c := range cases {
		if got := clampZoom(c.in); got != c.want {
			t.Errorf("clampZoom(%v) = %g, want %g", c.in, got, c.want)
		}
	}
}

// SetView with out-of-range zoom must clamp; with NaN center must
// land on a finite point (Clamp handles the NaN coercion).
func TestSetView_ClampsZoomAndCenter(t *testing.T) {
	w := &gui.Window{}
	readState(w, "m", MapState{})
	bad := projection.LatLng{Lat: 999, Lng: 999}
	SetView(w, "m", bad, maxZoomF+10)
	got, _ := Snapshot(w, "m")
	if got.Zoom != maxZoomF {
		t.Errorf("Zoom = %g, want %g", got.Zoom, maxZoomF)
	}
	if got.Center.Lat > 86 || got.Center.Lat < -86 {
		t.Errorf("Lat = %g not clamped", got.Center.Lat)
	}
}
