package jwks

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type item struct {
	data *JWK
	// expiration and lastUsed are in nanoseconds since the cache's epoch.
	// They are atomic rather than guarded by a lock: every token verification
	// touches the item, and verifications with the same key id would all
	// contend for that lock.
	//
	// expiration is when the key must be fetched from the JWKS endpoint again.
	// It is set when the key is fetched, and is not extended on use: a key
	// removed from the endpoint must stop being trusted within one TTL, however
	// often tokens signed with it keep coming.
	expiration atomic.Int64
	// lastUsed keeps a key which is still in use around after it expires, for
	// GetStale to fall back on when the JWKS endpoint can't be reached.
	lastUsed atomic.Int64
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

func (tc *TTLCache) newItem(key *JWK) *item {
	i := &item{data: key}
	now := tc.now()
	i.expiration.Store(now + int64(tc.ttl))
	i.lastUsed.Store(now)
	return i
}

func (tc *TTLCache) expired(i *item) bool {
	return i.expiration.Load() < tc.now()
}

// cleanup removes the items which expired and were not used for the TTL.
func (tc *TTLCache) cleanup() {
	tc.mu.Lock()
	now := tc.now()
	for key, item := range tc.items {
		if item.expiration.Load() < now && item.lastUsed.Load() < now-int64(tc.ttl) {
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
	tc.items[cacheKey] = tc.newItem(key)
	tc.mu.Unlock()
	return nil
}

// ReplacePrefix replaces the items under the cache keys starting with prefix
// by items: items missing from it are removed.
func (tc *TTLCache) ReplacePrefix(prefix string, items map[string]*JWK) error {
	tc.mu.Lock()
	for cacheKey := range tc.items {
		if strings.HasPrefix(cacheKey, prefix) {
			if _, ok := items[cacheKey]; !ok {
				delete(tc.items, cacheKey)
			}
		}
	}
	for cacheKey, key := range items {
		tc.items[cacheKey] = tc.newItem(key)
	}
	tc.mu.Unlock()
	return nil
}

// Get item by cacheKey. An expired item is not found.
func (tc *TTLCache) Get(cacheKey string) (*JWK, error) {
	tc.mu.RLock()
	item, ok := tc.items[cacheKey]
	if !ok || tc.expired(item) {
		tc.mu.RUnlock()
		return nil, ErrCacheNotFound
	}
	item.lastUsed.Store(tc.now())
	tc.mu.RUnlock()
	return item.data, nil
}

// GetStale gets item by cacheKey even if it expired, and lets Get find it for
// retryAfter more. Used when the key can't be fetched again, so that the next
// attempt to fetch it comes after retryAfter rather than on every Get.
func (tc *TTLCache) GetStale(cacheKey string, retryAfter time.Duration) (*JWK, error) {
	tc.mu.RLock()
	item, ok := tc.items[cacheKey]
	if !ok {
		tc.mu.RUnlock()
		return nil, ErrCacheNotFound
	}
	now := tc.now()
	item.lastUsed.Store(now)
	if item.expiration.Load() < now+int64(retryAfter) {
		item.expiration.Store(now + int64(retryAfter))
	}
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
