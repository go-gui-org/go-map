package mapview

import (
	"testing"

	"github.com/go-gui-org/go-map/projection"
)

var benchState = MapState{
	Center: projection.LatLng{Lat: 47.6, Lng: -122.3},
	Zoom:   12,
}

func BenchmarkComputeViewport(b *testing.B) {
	for b.Loop() {
		_ = computeViewport(800, 600, benchState)
	}
}

func BenchmarkMetersPerPixel(b *testing.B) {
	for b.Loop() {
		_ = metersPerPixel(45.0, 12.0)
	}
}

func BenchmarkNiceRound(b *testing.B) {
	for b.Loop() {
		_ = niceRound(4321.0)
	}
}

func BenchmarkZoomLabel(b *testing.B) {
	for b.Loop() {
		_ = zoomLabel(12.4)
	}
}

func BenchmarkComposeAttribution(b *testing.B) {
	layers := []Layer{
		{Source: attrSource{"© OpenStreetMap contributors"}, Kind: LayerKindBase, Visible: true, Opacity: 1},
		{Source: attrSource{"© Example WMS"}, Kind: LayerKindReference, Visible: true, Opacity: 1},
		{Source: attrSource{"© OpenStreetMap contributors"}, Kind: LayerKindReference, Visible: true, Opacity: 1},
	}
	b.ResetTimer()
	for b.Loop() {
		_ = composeAttribution(layers)
	}
}
