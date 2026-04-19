package tile

import (
	"sync"
	"testing"
)

func TestCachePutGet(t *testing.T) {
	c := NewCache(4)
	k := Coord{Z: 1, X: 0, Y: 0}
	c.Put(k, []byte("abc"))
	if got, ok := c.Get(k); !ok || string(got) != "abc" {
		t.Fatalf("want abc, got ok=%v data=%q", ok, got)
	}
}

// LRU eviction is Put-order only — Get does not promote entries.
// k1 is put first, so it is the LRU when k3 arrives; k2 survives.
func TestCacheEvictsLRU(t *testing.T) {
	c := NewCache(2)
	k1 := Coord{Z: 1, X: 0, Y: 0}
	k2 := Coord{Z: 1, X: 1, Y: 0}
	k3 := Coord{Z: 1, X: 0, Y: 1}

	c.Put(k1, []byte("1"))
	c.Put(k2, []byte("2"))
	// Get does not update eviction order; k1 remains the LRU.
	if _, ok := c.Get(k1); !ok {
		t.Fatal("k1 missing before eviction")
	}
	c.Put(k3, []byte("3"))

	if _, ok := c.Get(k1); ok {
		t.Error("k1 should have been evicted (oldest Put)")
	}
	if _, ok := c.Get(k2); !ok {
		t.Error("k2 should survive")
	}
	if _, ok := c.Get(k3); !ok {
		t.Error("k3 should be present")
	}
}

func TestCache_Len(t *testing.T) {
	c := NewCache(3)
	if c.Len() != 0 {
		t.Fatalf("empty cache Len = %d, want 0", c.Len())
	}
	c.Put(Coord{Z: 1, X: 0, Y: 0}, []byte("a"))
	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}
	c.Put(Coord{Z: 1, X: 1, Y: 0}, []byte("b"))
	c.Put(Coord{Z: 1, X: 2, Y: 0}, []byte("c"))
	c.Put(Coord{Z: 1, X: 3, Y: 0}, []byte("d")) // triggers eviction
	if c.Len() != 3 {
		t.Errorf("after eviction Len = %d, want 3", c.Len())
	}
}

// A second Put for an existing key must update data and keep Len stable.
func TestCache_PutExistingKeyRefreshes(t *testing.T) {
	c := NewCache(4)
	k := Coord{Z: 2, X: 1, Y: 1}
	c.Put(k, []byte("first"))
	c.Put(k, []byte("second"))
	got, ok := c.Get(k)
	if !ok {
		t.Fatal("key missing after second Put")
	}
	if string(got) != "second" {
		t.Errorf("data = %q, want \"second\"", got)
	}
	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1 after duplicate Put", c.Len())
	}
}

// NewCache with capacity ≤ 0 must clamp to 1 — Put+Get must round-trip.
func TestCache_CapacityClamp(t *testing.T) {
	for _, cap := range []int{0, -1, -100} {
		c := NewCache(cap)
		k := Coord{Z: 0, X: 0, Y: 0}
		c.Put(k, []byte("x"))
		if got, ok := c.Get(k); !ok || string(got) != "x" {
			t.Errorf("NewCache(%d): Get failed ok=%v data=%q", cap, ok, got)
		}
	}
}

// TestCache_ConcurrentGetPut runs mixed Get/Put under -race to detect
// data races introduced by the sync.RWMutex change.
func TestCache_ConcurrentGetPut(_ *testing.T) {
	c := NewCache(8)
	keys := []Coord{
		{Z: 1, X: 0, Y: 0}, {Z: 1, X: 1, Y: 0},
		{Z: 1, X: 0, Y: 1}, {Z: 1, X: 1, Y: 1},
	}
	for _, k := range keys {
		c.Put(k, []byte(k.String()))
	}

	var wg sync.WaitGroup
	const goroutines = 20
	for range goroutines {
		wg.Go(func() {
			for i, k := range keys {
				c.Get(k)
				c.Put(Coord{Z: 2, X: uint32(i), Y: 0}, []byte("v"))
			}
		})
	}
	wg.Wait()
}

func TestCoordValid(t *testing.T) {
	if !(Coord{Z: 0, X: 0, Y: 0}).Valid() {
		t.Error("0/0/0 valid")
	}
	if (Coord{Z: 1, X: 2, Y: 0}).Valid() {
		t.Error("1/2/0 out of range")
	}
	if (Coord{Z: 2, X: 3, Y: 3}).Valid() != true {
		t.Error("2/3/3 valid")
	}
}
