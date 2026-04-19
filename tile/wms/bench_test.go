package wms

import (
	"testing"

	"github.com/mike-ward/go-map/tile"
)

var benchCoord = tile.Coord{Z: 11, X: 328, Y: 715}

func BenchmarkBboxFor(b *testing.B) {
	for b.Loop() {
		_ = bboxFor(benchCoord)
	}
}
