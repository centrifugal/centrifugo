package redisnatsbroker

import (
	"github.com/centrifugal/centrifugo/v5/internal/natsbroker"

	"github.com/centrifugal/centrifuge"
)

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

// History ...
func (b *Broker) History(ch string, opts centrifuge.HistoryOptions) ([]*centrifuge.Publication, centrifuge.StreamPosition, error) {
	return b.redis.History(ch, opts)
}

// RemoveHistory ...
func (b *Broker) RemoveHistory(ch string) error {
	return b.redis.RemoveHistory(ch)
}
