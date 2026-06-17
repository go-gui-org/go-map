// Basic go-map demo: display an interactive OSM map centered on Seattle.
package main

import (
	"github.com/go-gui-org/go-gui/gui"
	"github.com/go-gui-org/go-gui/gui/backend"
	"github.com/go-gui-org/go-map/mapview"
	"github.com/go-gui-org/go-map/projection"
	"github.com/go-gui-org/go-map/tile"
)

// src is shared between the mapview Source and the window's
// ImageFetcher so tile downloads on the render path carry the same
// OSM-policy-compliant User-Agent as Fetch() would.
var src = tile.OSMWithUserAgent(
	"go-map-example/0 (https://github.com/go-gui-org/go-map)",
)

func main() {
	gui.SetTheme(gui.ThemeDark.WithBorders(true))
	cfg := gui.WindowCfg{
		Title:  "go-map",
		Width:  900,
		Height: 650,
		OnInit: func(w *gui.Window) {
			w.UpdateView(view)
		},
	}
	if f, ok := src.(tile.HTTPFetcher); ok {
		cfg.ImageFetcher = f.HTTPFetcher()
	}
	w := gui.NewWindow(cfg)
	backend.Run(w)
}

func view(w *gui.Window) gui.View {
	return mapview.FullWindow(w, mapview.Map(mapview.Cfg{
		ID:            "map",
		IDFocus:       1,
		Sizing:        gui.FillFill,
		InitialCenter: projection.LatLng{Lat: 47.6062, Lng: -122.3321},
		InitialZoom:   11,
		Source:        src,
		A11YLabel:     "Seattle street map",
	}))
}
