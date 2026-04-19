package tile

import (
	"container/list"
	"sync"
)

// Cache is a fixed-capacity LRU of encoded tile bytes keyed by Coord.
// Safe for concurrent use.
//
// LRU ordering reflects Put timestamps only — Get does not promote
// entries. Under tile workloads, reads are dominated by cache hits from
// a recently panned viewport (all Puts), so read-promotion has
// negligible impact on hit rate while removing it allows Get to use a
// read lock and eliminates contention during concurrent tile fetches.
type Cache struct {
	mu    sync.RWMutex
	cap   int
	items map[Coord]*list.Element
	order *list.List
}

type cacheEntry struct {
	key  Coord
	data []byte
}

// NewCache returns an LRU cache holding up to capacity tiles. Capacity
// less than 1 defaults to 1.
func NewCache(capacity int) *Cache {
	if capacity < 1 {
		capacity = 1
	}
	return &Cache{
		cap:   capacity,
		items: make(map[Coord]*list.Element, capacity),
		order: list.New(),
	}
}

// Get returns the cached bytes for key. ok is false if not present.
func (c *Cache) Get(key Coord) (data []byte, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if el, hit := c.items[key]; hit {
		return el.Value.(*cacheEntry).data, true
	}
	return nil, false
}

// Put stores data under key, evicting the LRU entry if at capacity.
// Storing an existing key refreshes it.
func (c *Cache) Put(key Coord, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, hit := c.items[key]; hit {
		el.Value.(*cacheEntry).data = data
		c.order.MoveToFront(el)
		return
	}
	if c.order.Len() >= c.cap {
		back := c.order.Back()
		if back != nil {
			c.order.Remove(back)
			delete(c.items, back.Value.(*cacheEntry).key)
		}
	}
	el := c.order.PushFront(&cacheEntry{key: key, data: data})
	c.items[key] = el
}

// Len returns the number of entries currently held.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.order.Len()
}
