package mapview

import (
	"math"
	"testing"

	"github.com/mike-ward/go-gui/gui"
	"github.com/mike-ward/go-map/projection"
)

// Map factory must reject empty Cfg.ID — the registry key is the
// only thing that ties state to a widget, so silently accepting ""
// would have multiple maps share state.
func TestMap_PanicsOnEmptyID(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Map(Cfg{}) did not panic")
		}
	}()
	_ = Map(Cfg{})
}

// InitialZoom > maxZoom must clamp at construction so the seed
// (and therefore the Home key) lands inside the renderable range.
func TestMap_ClampsInitialZoom(t *testing.T) {
	v := Map(Cfg{ID: "x", InitialZoom: maxZoom + 10})
	mv, ok := v.(*mapView)
	if !ok {
		t.Fatalf("Map returned %T, want *mapView", v)
	}
	if mv.cfg.InitialZoom != maxZoom {
		t.Errorf("InitialZoom = %d, want %d", mv.cfg.InitialZoom, maxZoom)
	}
}

// NaN coordinates in InitialCenter must be neutralized at
// construction; otherwise the first frame seeds the registry with
// NaN and every subsequent computation propagates it.
func TestMap_SanitizesInitialCenterNaN(t *testing.T) {
	v := Map(Cfg{
		ID:            "x",
		InitialCenter: projection.LatLng{Lat: math.NaN(), Lng: math.NaN()},
		InitialZoom:   5,
	})
	mv := v.(*mapView)
	if math.IsNaN(mv.cfg.InitialCenter.Lat) ||
		math.IsNaN(mv.cfg.InitialCenter.Lng) {
		t.Errorf("InitialCenter still contains NaN: %+v",
			mv.cfg.InitialCenter)
	}
}

// Zero-value Cfg (other than ID) must populate sensible defaults so
// a minimal Map(Cfg{ID:"x"}) renders without further setup.
func TestMap_DefaultsAppliedOnZeroCfg(t *testing.T) {
	v := Map(Cfg{ID: "x"})
	mv := v.(*mapView)
	if mv.cfg.Sizing != gui.FillFill {
		t.Errorf("Sizing = %+v, want FillFill", mv.cfg.Sizing)
	}
	if !mv.cfg.Background.IsSet() {
		t.Error("Background not set to default")
	}
	if mv.cfg.InitialZoom == 0 {
		t.Error("InitialZoom still 0; expected default seed")
	}
}
