// Package tile provides tile coordinates, tile sources, and a small
// LRU cache for slippy-tile map widgets.
package tile

import (
	"context"
	"errors"
	"strconv"
)

// Coord is a slippy-tile address: zoom, column, row.
// Row 0 is the northernmost row at the given zoom.
type Coord struct {
	Z uint32
	X uint32
	Y uint32
}

// String returns the canonical {z}/{x}/{y} form used in most tile URLs.
func (c Coord) String() string {
	var buf [32]byte
	b := strconv.AppendUint(buf[:0], uint64(c.Z), 10)
	b = append(b, '/')
	b = strconv.AppendUint(b, uint64(c.X), 10)
	b = append(b, '/')
	b = strconv.AppendUint(b, uint64(c.Y), 10)
	return string(b)
}

// Valid reports whether c is a legal tile address at zoom Z. The valid
// range for both X and Y is [0, 2^Z). Computed as uint32(1)<<c.Z, which
// is 0 when Z >= 32 (Go integer shift: a shift count >= the type's bit
// width yields 0 — defined, not undefined behaviour). Valid therefore
// returns false for Z >= 32, matching the practical ceiling of every
// supported tile provider. This is correct, not a bug.
func (c Coord) Valid() bool {
	n := uint32(1) << c.Z
	return c.X < n && c.Y < n
}

// ErrNotFound is returned by a Source when a tile does not exist.
var ErrNotFound = errors.New("tile: not found")

// Source supplies encoded tile image bytes for a Coord.
// Implementations must be safe for concurrent use.
type Source interface {
	// Fetch returns encoded image bytes (PNG/JPEG/WebP). The caller
	// owns the returned slice.
	Fetch(ctx context.Context, c Coord) ([]byte, error)

	// URL returns a string the rendering layer can pass to
	// gui.DrawContext.Image. For HTTP sources this is the tile URL;
	// for future offline sources it may be a data: URL or a local
	// path. Empty string means the Source cannot render through the
	// URL path and the caller must use Fetch instead.
	URL(c Coord) string

	// Attribution returns a short, human-readable credit string to be
	// rendered by the map widget. Required by most tile providers.
	Attribution() string

	// MaxZoom returns the highest zoom level this source serves.
	MaxZoom() uint32
}
