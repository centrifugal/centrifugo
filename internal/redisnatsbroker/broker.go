package redisnatsbroker

import (
	"github.com/centrifugal/centrifugo/v6/internal/natsbroker"

	"github.com/centrifugal/centrifuge"
)

// Broker is a combination of centrifuge.RedisBroker and NatsBroker. Redis is used for history
// operations and/or idempotent result cache. Nats is used for PUB/SUB part. The important limitation
// is that publications to this Broker must be sequential for the same channel. Otherwise, we
// can't guarantee message ordering and stable behavior of clients with auto recovery on. The
// benefit is more efficient fan-in in Nats PUB/SUB. Also, this allows scaling Redis Cluster
// without PUB/SUB scalability restrictions.
// This is EXPERIMENTAL.
type Broker struct {
	*natsbroker.NatsBroker
	redis *centrifuge.RedisBroker
}

func New(nats *natsbroker.NatsBroker, redis *centrifuge.RedisBroker) (*Broker, error) {
	return &Broker{
		NatsBroker: nats,
		redis:      redis,
	}, nil
}

func (b *Broker) Publish(ch string, data []byte, opts centrifuge.PublishOptions) (centrifuge.StreamPosition, bool, error) {
	if !b.NatsBroker.IsSupportedPublishChannel(ch) {
		// Do not support wildcard subscriptions just like natsbroker.NatsBroker.
		return centrifuge.StreamPosition{}, false, centrifuge.ErrorBadRequest
	}
	if opts.IdempotencyKey != "" || (opts.HistorySize > 0 && opts.HistoryTTL > 0) {
		sp, fromCache, err := b.redis.Publish(ch, data, opts)
		if err != nil {
			return sp, fromCache, err
		}
		if fromCache {
			return sp, true, nil
		}
		_ = b.NatsBroker.PublishWithStreamPosition(ch, data, opts, sp)
		return sp, fromCache, nil
	}
	return b.NatsBroker.Publish(ch, data, opts)
}

// History ...
func (b *Broker) History(ch string, opts centrifuge.HistoryOptions) ([]*centrifuge.Publication, centrifuge.StreamPosition, error) {
	return b.redis.History(ch, opts)
}

// RemoveHistory ...
func (b *Broker) RemoveHistory(ch string) error {
	return b.redis.RemoveHistory(ch)
}
