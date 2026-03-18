package pubsub

import (
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
)

// Config configures PubSub creation.
type Config struct {
	// Type selects the backend: "redis" (default) or "nats".
	Type string
	// Channel is the notification channel/subject name. Prefix is applied automatically.
	Channel string
	// Redis connection config with prefix. Used when Type is "redis".
	Redis configtypes.RedisPrefixed
	// Nats connection config with prefix. Used when Type is "nats".
	Nats configtypes.NatsPrefixed
}

// New creates a PubSub instance from config.
func New(cfg Config) (PubSub, error) {
	switch cfg.Type {
	case "", "redis":
		return NewRedisPubSub(cfg.Redis, cfg.Channel)
	case "nats":
		return NewNatsPubSub(cfg.Nats, cfg.Channel)
	default:
		return nil, fmt.Errorf("unknown pubsub type: %q", cfg.Type)
	}
}
