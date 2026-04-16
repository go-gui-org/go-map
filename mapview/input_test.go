package mapview

import (
	"math"
	"testing"

	"github.com/mike-ward/go-map/projection"
)

// TestZoomToward_Invariant: the LatLng under the cursor must not
// move when zoom changes. This is the defining property of
// zoom-to-cursor pan.
func TestZoomToward_Invariant(t *testing.T) {
	s := MapState{
		Center: projection.LatLng{Lat: 47.6062, Lng: -122.3321},
		Zoom:   11,
	}
	widgetW, widgetH := float32(900), float32(650)

	cases := []struct {
		name       string
		cx, cy     float32
		newZoom    uint32
		tolDegrees float64
	}{
		{"center_zoom_in", 450, 325, 12, 1e-9},
		{"center_zoom_out", 450, 325, 10, 1e-9},
		{"top_left_zoom_in", 50, 50, 13, 1e-6},
		{"bottom_right_zoom_out", 850, 600, 9, 1e-6},
		{"off_center_in", 700, 200, 14, 1e-6},
		{"two_steps_in", 200, 400, 13, 1e-6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// LatLng under the cursor BEFORE the zoom change.
			oldPt := projection.Project(s.Center, s.Zoom)
			cursorPxOld := projection.Point{
				X: oldPt.X + float64(c.cx-widgetW/2),
				Y: oldPt.Y + float64(c.cy-widgetH/2),
			}
			want := projection.Unproject(cursorPxOld, s.Zoom)

			// Compute new center, then resolve the LatLng at the SAME
			// screen position under the new zoom.
			newCtr := zoomToward(s, c.newZoom,
				c.cx, c.cy, widgetW, widgetH)
			newPt := projection.Project(newCtr, c.newZoom)
			cursorPxNew := projection.Point{
				X: newPt.X + float64(c.cx-widgetW/2),
				Y: newPt.Y + float64(c.cy-widgetH/2),
			}
			got := projection.Unproject(cursorPxNew, c.newZoom)

			if dLat := math.Abs(got.Lat - want.Lat); dLat > c.tolDegrees {
				t.Errorf("Lat drift %g > tol %g (got %v, want %v)",
					dLat, c.tolDegrees, got.Lat, want.Lat)
			}
			if dLng := math.Abs(got.Lng - want.Lng); dLng > c.tolDegrees {
				t.Errorf("Lng drift %g > tol %g (got %v, want %v)",
					dLng, c.tolDegrees, got.Lng, want.Lng)
			}
		})
	}
}

// TestZoomToward_SameZoomNoop: zooming to the current zoom level
// must leave the center unchanged.
func TestZoomToward_SameZoomNoop(t *testing.T) {
	s := MapState{
		Center: projection.LatLng{Lat: 40.7128, Lng: -74.0060},
		Zoom:   10,
	}
	got := zoomToward(s, s.Zoom, 400, 300, 800, 600)
	if math.Abs(got.Lat-s.Center.Lat) > 1e-9 {
		t.Errorf("Lat changed on no-op zoom: got %v, want %v",
			got.Lat, s.Center.Lat)
	}
	if math.Abs(got.Lng-s.Center.Lng) > 1e-9 {
		t.Errorf("Lng changed on no-op zoom: got %v, want %v",
			got.Lng, s.Center.Lng)
	}
}

// TestZoomToward_CursorAtCenter: when the cursor sits exactly at the
// canvas center, the map center must not move regardless of zoom
// delta.
func TestZoomToward_CursorAtCenter(t *testing.T) {
	s := MapState{
		Center: projection.LatLng{Lat: 51.5074, Lng: -0.1278},
		Zoom:   8,
	}
	widgetW, widgetH := float32(1024), float32(768)
	for _, newZ := range []uint32{5, 7, 9, 12, 16} {
		got := zoomToward(s, newZ, widgetW/2, widgetH/2,
			widgetW, widgetH)
		// Mercator Unproject(Project(p, z), z) preserves within ~1e-12
		// at these latitudes; allow 1e-9.
		if math.Abs(got.Lat-s.Center.Lat) > 1e-9 {
			t.Errorf("z=%d: Lat drift got %v, want %v",
				newZ, got.Lat, s.Center.Lat)
		}
		if math.Abs(got.Lng-s.Center.Lng) > 1e-9 {
			t.Errorf("z=%d: Lng drift got %v, want %v",
				newZ, got.Lng, s.Center.Lng)
		}
	}
}
