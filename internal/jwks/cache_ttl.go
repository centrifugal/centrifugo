package jwks

import (
	"sync"
	"sync/atomic"
	"time"
)

type item struct {
	data *JWK
	// expiration is in nanoseconds since the cache's epoch. It is atomic
	// rather than guarded by a lock: every token verification touches the
	// item, and verifications with the same key id would all contend for
	// that lock.
	expiration atomic.Int64
}

// TTLCache is a TTL bases in-memory cache.
type TTLCache struct {
	mu       sync.RWMutex
	epoch    time.Time
	ttl      time.Duration
	stop     chan struct{}
	stopOnce sync.Once
	items    map[string]*item
}

// NewTTLCache returns a new instance of ttl cache.
func NewTTLCache(ttl time.Duration) *TTLCache {
	cache := &TTLCache{
		epoch: time.Now(),
		ttl:   ttl,
		stop:  make(chan struct{}),
		items: make(map[string]*item),
	}
	cache.run()
	return cache
}

// now returns the time since the cache's epoch. It reads the monotonic
// clock, so wall clock changes don't move expirations.
func (tc *TTLCache) now() int64 {
	return int64(time.Since(tc.epoch))
}

func (tc *TTLCache) touch(i *item) {
	i.expiration.Store(tc.now() + int64(tc.ttl))
}

func (tc *TTLCache) expired(i *item) bool {
	return i.expiration.Load() < tc.now()
}

func (tc *TTLCache) cleanup() {
	tc.mu.Lock()
	for key, item := range tc.items {
		if tc.expired(item) {
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
	tc.touch(item)
	tc.items[cacheKey] = item
	tc.mu.Unlock()
	return nil
}

// Get item by cacheKey.
func (tc *TTLCache) Get(cacheKey string) (*JWK, error) {
	tc.mu.RLock()
	item, ok := tc.items[cacheKey]
	if !ok || tc.expired(item) {
		tc.mu.RUnlock()
		return nil, ErrCacheNotFound
	}
	tc.touch(item)
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
