package docsync

import (
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"

	"github.com/Yiling-J/theine-go"
)

type Cache struct {
	cache *theine.Cache[string, *proxyproto.Document]
}

func NewCache() (*Cache, error) {
	cache, err := theine.NewBuilder[string, *proxyproto.Document](1000).Build()
	if err != nil {
		return nil, err
	}
	return &Cache{
		cache: cache,
	}, nil
}

func (c *Cache) Get(key string) (*proxyproto.Document, bool, error) {
	value, ok := c.cache.Get(key)
	if !ok {
		return nil, false, nil
	}
	return value, true, nil
}

func (c *Cache) Set(key string, value *proxyproto.Document) error {
	c.cache.SetWithTTL(key, value, 1, time.Second*10)
	return nil
}
