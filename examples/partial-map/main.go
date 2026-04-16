// Partial-map demo: map alongside a detail panel that reads
// mapview.Snapshot each frame to display the live viewport.
// Demonstrates the partial-mapplication layout pattern.
package main

import (
	"fmt"

	"github.com/mike-ward/go-gui/gui"
	"github.com/mike-ward/go-gui/gui/backend"
	"github.com/mike-ward/go-map/mapview"
	"github.com/mike-ward/go-map/projection"
	"github.com/mike-ward/go-map/tile"
)

const mapID = "partial-map"

var src = tile.OSMWithUserAgent(
	"go-map-partial-example/0 (https://github.com/mike-ward/go-map)",
)

func main() {
	gui.SetTheme(gui.ThemeDarkBordered)
	cfg := gui.WindowCfg{
		Title:  "go-map — partial",
		Width:  1100,
		Height: 700,
		OnInit: func(w *gui.Window) { w.UpdateView(view) },
	}
	if f, ok := src.(tile.HTTPFetcher); ok {
		cfg.ImageFetcher = f.HTTPFetcher()
	}
	backend.Run(gui.NewWindow(cfg))
}

func view(w *gui.Window) gui.View {
	ww, wh := w.WindowSize()
	return gui.Row(gui.ContainerCfg{
		Width:   float32(ww),
		Height:  float32(wh),
		Sizing:  gui.FixedFixed,
		Padding: gui.Some(gui.Padding{}),
		Content: []gui.View{
			mapview.Map(mapview.Cfg{
				ID:            mapID,
				IDFocus:       1,
				Sizing:        gui.FillFill,
				InitialCenter: projection.LatLng{Lat: 40.7128, Lng: -74.0060},
				InitialZoom:   12,
				Source:        src,
				A11YLabel:     "New York City street map",
			}),
			detailPanel(w),
		},
	})
}

// detailPanel reads the current map snapshot and renders the live
// center / zoom. Re-runs each frame; no callback wiring required.
func detailPanel(w *gui.Window) gui.View {
	s, ok := mapview.Snapshot(w, mapID)
	body := []gui.View{gui.Text(gui.TextCfg{Text: "Viewport", Hero: true})}
	if !ok {
		body = append(body, gui.Text(gui.TextCfg{Text: "(rendering…)"}))
	} else {
		body = append(body,
			gui.Text(gui.TextCfg{Text: fmt.Sprintf("Zoom: %d", s.Zoom)}),
			gui.Text(gui.TextCfg{Text: fmt.Sprintf("Lat: %.4f°", s.Center.Lat)}),
			gui.Text(gui.TextCfg{Text: fmt.Sprintf("Lng: %.4f°", s.Center.Lng)}),
		)
	}
	return gui.Column(gui.ContainerCfg{
		Sizing:  gui.FixedFill,
		Width:   240,
		Padding: gui.Some(gui.Padding{Left: 12, Right: 12, Top: 12, Bottom: 12}),
		Spacing: gui.Some[float32](6),
		Content: body,
	})
}
