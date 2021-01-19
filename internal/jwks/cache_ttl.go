package jwks

import (
	"context"
	"sync"
	"time"
)

type item struct {
	sync.RWMutex
	data       *JWK
	expiration *time.Time
}

func (i *item) touch(d time.Duration) {
	i.Lock()
	exp := time.Now().Add(d)
	i.expiration = &exp
	i.Unlock()
}

func (i *item) expired() bool {
	i.RLock()
	res := true
	if i.expiration != nil {
		res = i.expiration.Before(time.Now())
	}
	i.RUnlock()
	return res
}

// TTLCache is a TTL bases in-memory cache.
type TTLCache struct {
	mu    sync.RWMutex
	ttl   time.Duration
	stop  chan struct{}
	items map[string]*item
}

// NewTTLCache returns a new instance of ttl cache.
func NewTTLCache(ttl time.Duration) *TTLCache {
	cache := &TTLCache{
		ttl:   ttl,
		stop:  make(chan struct{}),
		items: make(map[string]*item),
	}
	cache.run()
	return cache
}

func (tc *TTLCache) cleanup() {
	tc.mu.Lock()
	for key, item := range tc.items {
		if item.expired() {
			delete(tc.items, key)
		}
	}
	tc.mu.Unlock()
}

func (tc *TTLCache) run() {
	d := tc.ttl
	if d < time.Second {
		d = time.Second
	}

	ticker := time.Tick(d)
	go func() {
		for {
			select {
			case <-ticker:
				tc.cleanup()
			case <-tc.stop:
				return
			}
		}
	}()
}

// Add item into cache.
func (tc *TTLCache) Add(_ context.Context, key *JWK) error {
	tc.mu.Lock()
	item := &item{data: key}
	item.touch(tc.ttl)
	tc.items[key.Kid] = item
	tc.mu.Unlock()
	return nil
}

// Get item by key.
func (tc *TTLCache) Get(_ context.Context, kid string) (*JWK, error) {
	tc.mu.RLock()
	item, ok := tc.items[kid]
	if !ok || item.expired() {
		tc.mu.RUnlock()
		return nil, ErrCacheNotFound
	}
	item.touch(tc.ttl)
	tc.mu.RUnlock()
	return item.data, nil
}

// Remove item by key.
func (tc *TTLCache) Remove(_ context.Context, kid string) error {
	tc.mu.Lock()
	delete(tc.items, kid)
	tc.mu.Unlock()
	return nil
}

// Contains checks item on existence.
func (tc *TTLCache) Contains(_ context.Context, kid string) (bool, error) {
	tc.mu.RLock()
	_, ok := tc.items[kid]
	tc.mu.RUnlock()
	return ok, nil
}

// Len returns current size of cache.
func (tc *TTLCache) Len(_ context.Context) (int, error) {
	tc.mu.RLock()
	n := len(tc.items)
	tc.mu.RUnlock()
	return n, nil
}

// Purge deletes all items.
func (tc *TTLCache) Purge(_ context.Context) error {
	tc.mu.Lock()
	tc.items = map[string]*item{}
	tc.mu.Unlock()
	return nil
}
