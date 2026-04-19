package tile

import "testing"

var benchCoord = Coord{Z: 11, X: 328, Y: 715}

func BenchmarkCoordString(b *testing.B) {
	for b.Loop() {
		_ = benchCoord.String()
	}
}

func BenchmarkBuildTileURL(b *testing.B) {
	for b.Loop() {
		_ = buildTileURL(osmURLPrefix, benchCoord)
	}
}

func BenchmarkCacheGet(b *testing.B) {
	c := NewCache(256)
	c.Put(benchCoord, []byte("data"))
	b.ResetTimer()
	for b.Loop() {
		_, _ = c.Get(benchCoord)
	}
}

func BenchmarkCacheGetParallel(b *testing.B) {
	c := NewCache(256)
	c.Put(benchCoord, []byte("data"))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = c.Get(benchCoord)
		}
	})
}

func BenchmarkCachePut(b *testing.B) {
	c := NewCache(4) // tiny cap forces eviction on every Put
	data := []byte("data")
	b.ResetTimer()
	for i := range b.N {
		c.Put(Coord{Z: 11, X: uint32(i), Y: 0}, data)
	}
}
