package mapview

import (
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

// SetZoom values above maxZoom must clamp; otherwise consumers can
// permanently park the viewport at an unreachable zoom level.
func TestSetZoom_ClampsAtMaxZoom(t *testing.T) {
	w := &gui.Window{}
	readState(w, "m", MapState{Zoom: 10})
	SetZoom(w, "m", maxZoom+5)
	got, _ := Snapshot(w, "m")
	if got.Zoom != maxZoom {
		t.Errorf("Zoom = %d, want %d", got.Zoom, maxZoom)
	}
}

// SetView with out-of-range zoom must clamp; with NaN center must
// land on a finite point (Clamp handles the NaN coercion).
func TestSetView_ClampsZoomAndCenter(t *testing.T) {
	w := &gui.Window{}
	readState(w, "m", MapState{})
	bad := projection.LatLng{Lat: 999, Lng: 999}
	SetView(w, "m", bad, maxZoom+10)
	got, _ := Snapshot(w, "m")
	if got.Zoom != maxZoom {
		t.Errorf("Zoom = %d, want %d", got.Zoom, maxZoom)
	}
	if got.Center.Lat > 86 || got.Center.Lat < -86 {
		t.Errorf("Lat = %g not clamped", got.Center.Lat)
	}
}
