// Package natsbroker defines custom Nats Broker for Centrifuge library.
package natsbroker

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/centrifugal/centrifuge"

	"github.com/nats-io/nats"
)

type (
	// channelID is unique channel identificator in Nats.
	channelID string
)

// Config of NatsEngine.
type Config struct {
	Servers string
	Prefix  string
}

// NatsBroker is a broker on top of Nats messaging system.
type NatsBroker struct {
	node   *centrifuge.Node
	config Config

	nc           *nats.Conn
	subsMu       sync.Mutex
	subs         map[channelID]*nats.Subscription
	eventHandler centrifuge.BrokerEventHandler
}

// New creates NatsEngine.
func New(n *centrifuge.Node, conf Config) (*NatsBroker, error) {
	b := &NatsBroker{
		node:   n,
		config: conf,
		subs:   make(map[channelID]*nats.Subscription),
	}
	return b, nil
}

func (b *NatsBroker) controlChannel() channelID {
	return channelID(b.config.Prefix + ".control")
}

func (b *NatsBroker) clientChannel(ch string) channelID {
	return channelID(b.config.Prefix + ".client." + ch)
}

// Run runs engine after node initialized.
func (b *NatsBroker) Run(h centrifuge.BrokerEventHandler) error {
	b.eventHandler = h
	servers := b.config.Servers
	if servers == "" {
		servers = nats.DefaultURL
	}
	nc, err := nats.Connect(servers, nats.ReconnectBufSize(-1), nats.MaxReconnects(math.MaxInt64))
	if err != nil {
		return err
	}
	_, err = nc.Subscribe(string(b.controlChannel()), b.handleControl)
	if err != nil {
		return err
	}
	b.nc = nc
	b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, fmt.Sprintf("Nats Broker connected to: %s", servers)))
	return nil
}

// Close is not implemented.
func (b *NatsBroker) Close(ctx context.Context) error {
	return nil
}

// Publish - see Engine interface description.
func (b *NatsBroker) Publish(ch string, pub *centrifuge.Publication, opts *centrifuge.ChannelOptions) error {
	data, err := pub.Marshal()
	if err != nil {
		return err
	}
	push := &centrifuge.Push{
		Type:    centrifuge.PushTypePublication,
		Channel: ch,
		Data:    data,
	}
	byteMessage, err := push.Marshal()
	if err != nil {
		return err
	}
	return b.nc.Publish(string(b.clientChannel(ch)), byteMessage)
}

// PublishJoin - see Engine interface description.
func (b *NatsBroker) PublishJoin(ch string, join *centrifuge.Join, opts *centrifuge.ChannelOptions) error {
	data, err := join.Marshal()
	if err != nil {
		return err
	}
	push := &centrifuge.Push{
		Type:    centrifuge.PushTypeJoin,
		Channel: ch,
		Data:    data,
	}
	byteMessage, err := push.Marshal()
	if err != nil {
		return err
	}
	return b.nc.Publish(string(b.clientChannel(ch)), byteMessage)
}

// PublishLeave - see Engine interface description.
func (b *NatsBroker) PublishLeave(ch string, leave *centrifuge.Leave, opts *centrifuge.ChannelOptions) error {
	data, err := leave.Marshal()
	if err != nil {
		return err
	}
	push := &centrifuge.Push{
		Type:    centrifuge.PushTypeLeave,
		Channel: ch,
		Data:    data,
	}
	byteMessage, err := push.Marshal()
	if err != nil {
		return err
	}
	return b.nc.Publish(string(b.clientChannel(ch)), byteMessage)
}

// PublishControl - see Engine interface description.
func (b *NatsBroker) PublishControl(data []byte) error {
	return b.nc.Publish(string(b.controlChannel()), data)
}

func (b *NatsBroker) handleClientMessage(chID channelID, data []byte) error {
	var push centrifuge.Push
	err := push.Unmarshal(data)
	if err != nil {
		return err
	}
	switch push.Type {
	case centrifuge.PushTypePublication:
		var pub centrifuge.Publication
		err := pub.Unmarshal(push.Data)
		if err != nil {
			return err
		}
		b.eventHandler.HandlePublication(push.Channel, &pub)
	case centrifuge.PushTypeJoin:
		var join centrifuge.Join
		err := join.Unmarshal(push.Data)
		if err != nil {
			return err
		}
		b.eventHandler.HandleJoin(push.Channel, &join)
	case centrifuge.PushTypeLeave:
		var leave centrifuge.Leave
		err := leave.Unmarshal(push.Data)
		if err != nil {
			return err
		}
		b.eventHandler.HandleLeave(push.Channel, &leave)
	default:
	}
	return nil
}

func (b *NatsBroker) handleClient(m *nats.Msg) {
	b.handleClientMessage(channelID(m.Subject), m.Data)
}

func (b *NatsBroker) handleControl(m *nats.Msg) {
	b.eventHandler.HandleControl(m.Data)
}

// Subscribe - see Engine interface description.
func (b *NatsBroker) Subscribe(ch string) error {
	if strings.Contains(ch, "*") {
		// Do not support wildcard subscriptions.
		return centrifuge.ErrorBadRequest
	}
	b.subsMu.Lock()
	defer b.subsMu.Unlock()
	subClient, err := b.nc.Subscribe(string(b.clientChannel(ch)), b.handleClient)
	if err != nil {
		return err
	}
	b.subs[b.clientChannel(ch)] = subClient
	return nil
}

// Unsubscribe - see Engine interface description.
func (b *NatsBroker) Unsubscribe(ch string) error {
	b.subsMu.Lock()
	defer b.subsMu.Unlock()
	if sub, ok := b.subs[b.clientChannel(ch)]; ok {
		sub.Unsubscribe()
		delete(b.subs, b.clientChannel(ch))
	}
	return nil
}

// Channels - see Engine interface description.
func (b *NatsBroker) Channels() ([]string, error) {
	return nil, nil
}
