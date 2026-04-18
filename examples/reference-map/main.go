// Reference-map demo: a detail map alongside a locator (overview)
// map. The locator draws a rectangle showing the detail map's
// visible extent; clicking the locator recenters the detail map on
// the clicked point. The locator is independently pannable so the
// user can scout context around the detail viewport.
// Demonstrates shared-state linking between two mapview widgets.
package main

import (
	"github.com/mike-ward/go-gui/gui"
	"github.com/mike-ward/go-gui/gui/backend"
	"github.com/mike-ward/go-map/mapview"
	"github.com/mike-ward/go-map/projection"
	"github.com/mike-ward/go-map/tile"
)

const (
	detailID   = "ref-detail"
	overviewID = "ref-overview"
	sidebarW   = 300
	overviewH  = 240
	// Overview stays this many zoom levels wider than the detail map
	// so it reads as a locator, not a duplicate.
	overviewZoomDelta = 4
	viewportRectID    = "viewport-rect"
)

var src = tile.OSMWithUserAgent(
	"go-map-reference-example/0 (https://github.com/mike-ward/go-map)",
)

// Times Square — a dense, recognizable starting point.
var (
	initCenter = projection.LatLng{Lat: 40.7580, Lng: -73.9855}
	initZoom   = 13.0
)

func main() {
	gui.SetTheme(gui.ThemeDarkBordered)
	cfg := gui.WindowCfg{
		Title:  "go-map — reference",
		Width:  1200,
		Height: 720,
		OnInit: func(w *gui.Window) { w.UpdateView(view) },
	}
	if f, ok := src.(tile.HTTPFetcher); ok {
		cfg.ImageFetcher = f.HTTPFetcher()
	}
	backend.Run(gui.NewWindow(cfg))
}

func view(w *gui.Window) gui.View {
	ww, wh := w.WindowSize()
	detailW := float32(ww) - float32(sidebarW)
	detailH := float32(wh)

	// Rewrite the viewport-rectangle overlay on the locator every frame
	// to track detail pan / zoom. Idempotent registry writes when
	// nothing changed; no callback wiring needed.
	syncOverview(w, detailW, detailH)

	return gui.Row(gui.ContainerCfg{
		Width:   float32(ww),
		Height:  float32(wh),
		Sizing:  gui.FixedFixed,
		Padding: gui.Some(gui.Padding{}),
		Content: []gui.View{
			mapview.Map(mapview.Cfg{
				ID:            detailID,
				IDFocus:       1,
				Sizing:        gui.FillFill,
				InitialCenter: initCenter,
				InitialZoom:   initZoom,
				Source:        src,
				A11YLabel:     "Detail map of New York City",
			}),
			sidebar(),
		},
	})
}

func sidebar() gui.View {
	return gui.Column(gui.ContainerCfg{
		Sizing:  gui.FixedFill,
		Width:   float32(sidebarW),
		Padding: gui.Some(gui.Padding{Left: 8, Right: 8, Top: 8, Bottom: 8}),
		Spacing: gui.Some[float32](8),
		Content: []gui.View{
			gui.Text(gui.TextCfg{Text: "Locator", Hero: true}),
			mapview.Map(mapview.Cfg{
				ID:            overviewID,
				IDFocus:       2,
				Sizing:        gui.FillFixed,
				Height:        float32(overviewH),
				InitialCenter: initCenter,
				InitialZoom:   initZoom - overviewZoomDelta,
				Source:        src,
				A11YLabel:     "Overview locator map",
				// Clicking the locator jumps the detail map to that point.
				// A drag pans the locator itself (drag-vs-click threshold
				// suppresses accidental recenters).
				OnClick: func(w *gui.Window, ll projection.LatLng) {
					mapview.PanTo(w, detailID, ll)
				},
			}),
			gui.Text(gui.TextCfg{
				Mode: gui.TextModeWrap,
				Text: "Click locator to recenter the detail view. " +
					"Drag the locator to scout context; drag or zoom " +
					"the detail map to watch the rectangle follow.",
			}),
		},
	})
}

// syncOverview rewrites the viewport-rectangle overlay on the locator
// so it reflects the detail map's current visible extent. Runs every
// frame. The locator's own center and zoom are left alone — the user
// can pan it independently to explore surrounding context; clicking
// it recenters the detail map (see sidebar's OnClick).
func syncOverview(w *gui.Window, detailW, detailH float32) {
	s, ok := mapview.Snapshot(w, detailID)
	if !ok {
		return
	}
	// Detail may not yet have been laid out — don't reproject a zero
	// canvas (the rectangle would collapse to a point).
	if detailW <= 0 || detailH <= 0 {
		return
	}

	// Detail viewport extent in world pixels at its own zoom, then
	// unproject the four corners to LatLng. Using the detail map's
	// own zoom keeps the rectangle accurate regardless of the locator's
	// zoom offset.
	c := projection.ProjectF(s.Center, s.Zoom)
	hw := float64(detailW) / 2
	hh := float64(detailH) / 2
	ne := projection.UnprojectF(
		projection.Point{X: c.X + hw, Y: c.Y - hh}, s.Zoom)
	sw := projection.UnprojectF(
		projection.Point{X: c.X - hw, Y: c.Y + hh}, s.Zoom)

	// A single 4-corner ring cannot represent an antimeridian-wrapped
	// or larger-than-world viewport honestly. Drop the rectangle this
	// frame (and clear any stale one) rather than paint a nonsensical
	// band across the locator.
	lngSpan := ne.Lng - sw.Lng
	if lngSpan <= 0 || lngSpan >= 360 {
		mapview.RemoveOverlay(w, overviewID, viewportRectID)
		return
	}

	// Ring winds clockwise starting NW so the polygon renders with a
	// consistent orientation. Stroke only — semi-transparent fill so
	// the underlying locator tiles remain visible through the box.
	rect := &mapview.Polygon{
		PolyID: viewportRectID,
		Ring: []projection.LatLng{
			{Lat: ne.Lat, Lng: sw.Lng},
			{Lat: ne.Lat, Lng: ne.Lng},
			{Lat: sw.Lat, Lng: ne.Lng},
			{Lat: sw.Lat, Lng: sw.Lng},
		},
		FillColor:   gui.Color{R: 255, G: 212, B: 0, A: 48},
		StrokeColor: gui.Hex(0xFFD400),
		StrokeWidth: 2,
	}
	mapview.AddOverlay(w, overviewID, rect)
}
