package jwks

import (
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
	mu       sync.RWMutex
	ttl      time.Duration
	stop     chan struct{}
	stopOnce sync.Once
	items    map[string]*item
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

	ticker := time.NewTicker(d)
	go func() {
		for {
			select {
			case <-ticker.C:
				tc.cleanup()
			case <-tc.stop:
				ticker.Stop()
				return
			}
		}
	}()
}

// Add item into cache under the given cacheKey.
func (tc *TTLCache) Add(cacheKey string, key *JWK) error {
	tc.mu.Lock()
	item := &item{data: key}
	item.touch(tc.ttl)
	tc.items[cacheKey] = item
	tc.mu.Unlock()
	return nil
}

// Get item by cacheKey.
func (tc *TTLCache) Get(cacheKey string) (*JWK, error) {
	tc.mu.RLock()
	item, ok := tc.items[cacheKey]
	if !ok || item.expired() {
		tc.mu.RUnlock()
		return nil, ErrCacheNotFound
	}
	item.touch(tc.ttl)
	tc.mu.RUnlock()
	return item.data, nil
}

// Stop stops TTL cache.
func (tc *TTLCache) Stop() error {
	tc.stopOnce.Do(func() {
		close(tc.stop)
	})
	return nil
}

func (tc *TTLCache) remove(cacheKey string) error {
	tc.mu.Lock()
	delete(tc.items, cacheKey)
	tc.mu.Unlock()
	return nil
}

// Len returns current size of cache.
func (tc *TTLCache) Len() (int, error) {
	tc.mu.RLock()
	n := len(tc.items)
	tc.mu.RUnlock()
	return n, nil
}
