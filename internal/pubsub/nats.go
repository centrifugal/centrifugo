package pubsub

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/nats-io/nats.go"
)

// NatsPubSub implements PubSub using NATS.
type NatsPubSub struct {
	conn    *nats.Conn
	subject string
}

// NewNatsPubSub creates a new NATS-backed PubSub.
// The subject is constructed as prefix + subjectName.
func NewNatsPubSub(cfg configtypes.NatsEmptyPrefixed, subjectName string) (*NatsPubSub, error) {
	url := cfg.URL
	if url == "" {
		url = nats.DefaultURL
	}
	options := []nats.Option{
		nats.ReconnectBufSize(-1),
		nats.MaxReconnects(-1),
	}
	if dialTimeout := cfg.DialTimeout.ToDuration(); dialTimeout > 0 {
		options = append(options, nats.Timeout(dialTimeout))
	}
	if writeTimeout := cfg.WriteTimeout.ToDuration(); writeTimeout > 0 {
		options = append(options, nats.FlusherTimeout(writeTimeout))
	}
	if cfg.TLS.Enabled {
		tlsConfig, err := cfg.TLS.ToGoTLSConfig("nats_pubsub")
		if err != nil {
			return nil, fmt.Errorf("error creating NATS TLS config: %w", err)
		}
		options = append(options, nats.Secure(tlsConfig))
	}
	nc, err := nats.Connect(url, options...)
	if err != nil {
		return nil, fmt.Errorf("error connecting to NATS for pubsub: %w", err)
	}
	return &NatsPubSub{
		conn:    nc,
		subject: cfg.Prefix + subjectName,
	}, nil
}

// Publish sends data to the NATS subject.
func (n *NatsPubSub) Publish(_ context.Context, data []byte) error {
	return n.conn.Publish(n.subject, data)
}

// Subscribe listens on the NATS subject and calls handler for each message.
// Blocks until ctx is cancelled.
func (n *NatsPubSub) Subscribe(ctx context.Context, handler func(data []byte)) error {
	sub, err := n.conn.Subscribe(n.subject, func(msg *nats.Msg) {
		handler(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("error subscribing to NATS subject: %w", err)
	}
	defer func() { _ = sub.Unsubscribe() }()
	<-ctx.Done()
	return ctx.Err()
}

// Close closes the NATS connection.
func (n *NatsPubSub) Close() error {
	n.conn.Close()
	return nil
}
