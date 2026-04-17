package mapview

import (
	"math"

	"github.com/mike-ward/go-gui/gui"
	"github.com/mike-ward/go-map/projection"
	"github.com/mike-ward/go-map/tile"
)

// viewport holds the derived screen geometry for one frame: size in
// pixels, the world-pixel position of the center, and the tile range
// visible on screen. Computed once per OnDraw from MapState. Z is the
// fractional zoom; TileZ = floor(Z) is the integer tile level tiles
// are fetched at, and TileScale = 2^(Z-TileZ) is the per-tile draw
// scale that produces smooth intermediates between integer levels.
// The Zoom() method returns Z so viewport satisfies the overlay
// Projector interface without a wrapper type.
type viewport struct {
	W, H      float32 // canvas size in pixels
	Z         float64
	TileZ     uint32
	TileScale float64
	CtrPx     projection.Point // world-pixel coords of state.Center
	MinTX     int32
	MaxTX     int32
	MinTY     int32
	MaxTY     int32
	OriginX   float32 // world-pixel x of canvas top-left corner
	OriginY   float32
}

// computeViewport derives the screen → world mapping for the given
// canvas size and map state. Kept pure (no DrawContext) so viewport
// math is unit-testable without a running window. Tile-range math
// happens in world-pixel units at the fractional zoom; tileZ = floor(Z)
// is used only for the URL key and the scale factor.
func computeViewport(w, h float32, s MapState) viewport {
	vp := viewport{W: w, H: h, Z: s.Zoom}
	tileZ := uint32(math.Floor(s.Zoom))
	if tileZ > maxZoom {
		tileZ = maxZoom
	}
	vp.TileZ = tileZ
	vp.TileScale = math.Exp2(s.Zoom - float64(tileZ))
	vp.CtrPx = projection.ProjectF(s.Center, s.Zoom)
	vp.OriginX = float32(vp.CtrPx.X) - vp.W/2
	vp.OriginY = float32(vp.CtrPx.Y) - vp.H/2

	// Tile step in world-pixels at the fractional zoom equals TileSize
	// scaled by TileScale (i.e. tiles for tileZ cover scaledTs pixels
	// on screen). A tile-edge ≥ 0 is guaranteed by clampZoom keeping Z
	// non-negative and TileScale in [1, 2).
	scaledTs := float64(projection.TileSize) * vp.TileScale
	vp.MinTX = int32(math.Floor(float64(vp.OriginX) / scaledTs))
	vp.MinTY = int32(math.Floor(float64(vp.OriginY) / scaledTs))
	vp.MaxTX = int32(math.Floor(float64(vp.OriginX+vp.W) / scaledTs))
	vp.MaxTY = int32(math.Floor(float64(vp.OriginY+vp.H) / scaledTs))
	return vp
}

// wrapTileX maps a (possibly negative or out-of-range) tile-x index
// into [0, maxN) so viewports that straddle the antimeridian pull
// the correct tiles. maxN must be >= 1.
func wrapTileX(tx, maxN int32) uint32 {
	return uint32(((tx % maxN) + maxN) % maxN)
}

// tileScreenPos returns the top-left screen-pixel position of the
// given tile within the viewport. Tiles are drawn at TileSize scaled
// by TileScale (=2^(Z-TileZ)) so neighboring tiles meet at the
// fractional zoom. The spacing here uses the unrounded scale while
// drawTiles draws each tile at ceil(TileSize*TileScale) — the size is
// ceil'd to overlap neighbors by ≤1 px and suppress subpixel seams;
// the position stays unrounded so tiles still advance by their true
// projected extent. A refactor that "aligns" the two formulas would
// reintroduce the seams.
func (vp viewport) tileScreenPos(tx, ty int32) (x, y float32) {
	ts := float32(projection.TileSize) * float32(vp.TileScale)
	x = float32(tx)*ts - vp.OriginX
	y = float32(ty)*ts - vp.OriginY
	return
}

// screenToLatLng converts canvas pixel coords to geographic coords
// using the viewport's fractional zoom and origin.
func (vp viewport) screenToLatLng(sx, sy float32) projection.LatLng {
	return projection.UnprojectF(projection.Point{
		X: float64(vp.OriginX + sx),
		Y: float64(vp.OriginY + sy),
	}, vp.Z)
}

// LatLngToScreen projects p into canvas-pixel coords at the viewport's
// fractional zoom. Satisfies the overlay Projector interface so
// overlays can be given a viewport directly without an adapter type.
func (vp viewport) LatLngToScreen(p projection.LatLng) (x, y float32) {
	pt := projection.ProjectF(p, vp.Z)
	return float32(pt.X) - vp.OriginX, float32(pt.Y) - vp.OriginY
}

// MetersToPixels converts ground meters at the given latitude into
// pixels at the viewport zoom. Returns 0 for non-finite derivations.
func (vp viewport) MetersToPixels(lat, meters float64) float32 {
	mpp := metersPerPixel(lat, vp.Z)
	if !finitePositive(mpp) {
		return 0
	}
	return float32(meters / mpp)
}

// Zoom reports the viewport's fractional zoom level.
func (vp viewport) Zoom() float64 { return vp.Z }

// drawTiles renders the visible tile grid. Tiles with a URL from the
// Source render as gui.DrawContext.Image; sources without a URL (or
// no Source at all) fall back to a labeled placeholder checkerboard
// so pan/zoom is still usable. Tiles are fetched at integer TileZ and
// drawn at TileSize × TileScale pixels so fractional zoom fills the
// canvas without seams. ceil() on the scaled size suppresses subpixel
// gaps between neighboring tiles at the cost of ≤1 px overdraw on
// transparent tile edges — invisible on opaque OSM tiles.
func drawTiles(dc *gui.DrawContext, vp viewport, src tile.Source) {
	maxN := int32(1) << vp.TileZ
	ts := float32(math.Ceil(float64(projection.TileSize) * vp.TileScale))
	even := gui.Hex(0xE8E6E0)
	odd := gui.Hex(0xDCDAD3)
	border := gui.Hex(0xBDBAB3)
	labelStyle := gui.TextStyle{Size: 10, Color: gui.Hex(0x888888)}

	for ty := vp.MinTY; ty <= vp.MaxTY; ty++ {
		if ty < 0 || ty >= maxN {
			continue
		}
		for tx := vp.MinTX; tx <= vp.MaxTX; tx++ {
			wrapped := wrapTileX(tx, maxN)
			x, y := vp.tileScreenPos(tx, ty)

			var url string
			if src != nil {
				url = src.URL(tile.Coord{
					Z: vp.TileZ,
					X: wrapped,
					Y: uint32(ty),
				})
			}
			if url != "" {
				dc.Image(x, y, ts, ts, url, gui.Opt[float32]{}, even)
				continue
			}
			// Placeholder.
			c := even
			if (int32(wrapped)+ty)&1 == 1 {
				c = odd
			}
			dc.FilledRect(x, y, ts, ts, c)
			dc.Rect(x, y, ts, ts, border, 1)
			dc.Text(x+6, y+4,
				(tile.Coord{Z: vp.TileZ, X: wrapped, Y: uint32(ty)}).String(),
				labelStyle)
		}
	}
}
