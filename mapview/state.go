package mapview

import (
	"github.com/mike-ward/go-gui/gui"
	"github.com/mike-ward/go-map/projection"
)

// State-registry namespaces. Convention: "mapview.<purpose>", one
// namespace per distinct value type, all keyed by Cfg.ID. capMaps is
// the per-namespace map cap passed to gui.StateMap.
const (
	nsState     = "mapview.state"
	nsPan       = "mapview.pan"
	nsHover     = "mapview.hover"
	nsLastFired = "mapview.lastfired"
	nsScroll    = "mapview.scroll"
	capMaps     = 16
)

// MapState is the persistent per-map state held in the Window state
// registry. Accessed by the widget factory each frame and mutated by
// package-level helpers (PanTo, SetZoom, ...).
//
// Transient drag-tracking fields live on panState below to keep the
// snapshot type small.
type MapState struct {
	Center projection.LatLng
	Zoom   uint32
}

// panState tracks an in-progress drag pan. Stored in a separate
// namespace so MapState stays a clean snapshot.
type panState struct {
	Active    bool
	StartX    float32 // mouse down position (window coords)
	StartY    float32
	StartCtr  projection.LatLng // center at drag start
	StartZoom uint32
}

// lastFired records the MapState last passed to OnMove / OnZoomChange
// so the next frame can detect deltas. Set=false means "no baseline
// yet" and suppresses the synthetic first-frame change event.
type lastFired struct {
	State MapState
	Set   bool
}

// nsRead and nsWrite are the only state-registry primitives used by
// this package. Eight specialized read/write pairs collapsed into
// these two so callers can never accidentally bypass the namespace
// constants by reaching for gui.StateMap directly.
func nsRead[V any](w *gui.Window, ns, id string) V {
	var zero V
	return gui.StateReadOr[string, V](w, ns, id, zero)
}

func nsWrite[V any](w *gui.Window, ns, id string, v V) {
	gui.StateMap[string, V](w, ns, capMaps).Set(id, v)
}

// readState returns the current MapState for id, seeding it from seed
// if the registry has no entry yet. The seed branch is the reason
// readState exists — every other reader uses nsRead directly.
func readState(w *gui.Window, id string, seed MapState) MapState {
	sm := gui.StateMap[string, MapState](w, nsState, capMaps)
	if s, ok := sm.Get(id); ok {
		return s
	}
	sm.Set(id, seed)
	return seed
}

// Snapshot returns the current MapState for the map with the given
// ID. ok is false if the map has not yet rendered (in which case the
// returned MapState is the zero value); callers must check ok before
// trusting Center/Zoom — the zero value is a real point on the
// equator, not a sentinel.
func Snapshot(w *gui.Window, id string) (s MapState, ok bool) {
	return gui.StateMap[string, MapState](w, nsState, capMaps).Get(id)
}

// PanTo recenters the map on the given LatLng. Zoom is unchanged.
// No-op if the map ID has not yet rendered.
func PanTo(w *gui.Window, id string, c projection.LatLng) {
	sm := gui.StateMap[string, MapState](w, nsState, capMaps)
	s, ok := sm.Get(id)
	if !ok {
		return
	}
	s.Center = c.Clamp()
	sm.Set(id, s)
}

// SetZoom updates the zoom level, clamped to [0, maxZoom]. Center
// unchanged. No-op if the map ID has not yet rendered.
func SetZoom(w *gui.Window, id string, zoom uint32) {
	sm := gui.StateMap[string, MapState](w, nsState, capMaps)
	s, ok := sm.Get(id)
	if !ok {
		return
	}
	if zoom > maxZoom {
		zoom = maxZoom
	}
	s.Zoom = zoom
	sm.Set(id, s)
}

// SetView replaces both center and zoom atomically.
func SetView(w *gui.Window, id string, c projection.LatLng, zoom uint32) {
	sm := gui.StateMap[string, MapState](w, nsState, capMaps)
	s, ok := sm.Get(id)
	if !ok {
		return
	}
	if zoom > maxZoom {
		zoom = maxZoom
	}
	s.Center = c.Clamp()
	s.Zoom = zoom
	sm.Set(id, s)
}

// maxZoom is the global zoom ceiling. Tile sources may cap lower via
// their own MaxZoom(); input handlers consult that at call time.
const maxZoom uint32 = 22
