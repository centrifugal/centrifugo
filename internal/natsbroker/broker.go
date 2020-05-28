// Package natsbroker defines custom Nats Broker for Centrifuge library.
package natsbroker

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/nats-io/nats.go"
)

type (
	// channelID is unique channel identifier in Nats.
	channelID string
)

// Config of NatsBroker.
type Config struct {
	URL          string
	Prefix       string
	DialTimeout  time.Duration
	WriteTimeout time.Duration
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

// New creates NatsBroker.
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
	url := b.config.URL
	if url == "" {
		url = nats.DefaultURL
	}
	nc, err := nats.Connect(
		url,
		nats.ReconnectBufSize(-1),
		nats.MaxReconnects(math.MaxInt64),
		nats.Timeout(b.config.DialTimeout),
		nats.FlusherTimeout(b.config.WriteTimeout),
	)
	if err != nil {
		return fmt.Errorf("error connecting to %s: %w", url, err)
	}
	_, err = nc.Subscribe(string(b.controlChannel()), b.handleControl)
	if err != nil {
		return err
	}
	b.nc = nc
	b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, fmt.Sprintf("Nats Broker connected to: %s", url)))
	return nil
}

// Close is not implemented.
func (b *NatsBroker) Close(_ context.Context) error {
	return nil
}

// Publish - see Broker interface description.
func (b *NatsBroker) Publish(ch string, pub *protocol.Publication, _ *centrifuge.ChannelOptions) error {
	data, err := pub.Marshal()
	if err != nil {
		return err
	}
	push := &protocol.Push{
		Type:    protocol.PushTypePublication,
		Channel: ch,
		Data:    data,
	}
	byteMessage, err := push.Marshal()
	if err != nil {
		return err
	}
	return b.nc.Publish(string(b.clientChannel(ch)), byteMessage)
}

// PublishJoin - see Broker interface description.
func (b *NatsBroker) PublishJoin(ch string, join *protocol.Join, _ *centrifuge.ChannelOptions) error {
	data, err := join.Marshal()
	if err != nil {
		return err
	}
	push := &protocol.Push{
		Type:    protocol.PushTypeJoin,
		Channel: ch,
		Data:    data,
	}
	byteMessage, err := push.Marshal()
	if err != nil {
		return err
	}
	return b.nc.Publish(string(b.clientChannel(ch)), byteMessage)
}

// PublishLeave - see Broker interface description.
func (b *NatsBroker) PublishLeave(ch string, leave *protocol.Leave, _ *centrifuge.ChannelOptions) error {
	data, err := leave.Marshal()
	if err != nil {
		return err
	}
	push := &protocol.Push{
		Type:    protocol.PushTypeLeave,
		Channel: ch,
		Data:    data,
	}
	byteMessage, err := push.Marshal()
	if err != nil {
		return err
	}
	return b.nc.Publish(string(b.clientChannel(ch)), byteMessage)
}

// PublishControl - see Broker interface description.
func (b *NatsBroker) PublishControl(data []byte) error {
	return b.nc.Publish(string(b.controlChannel()), data)
}

func (b *NatsBroker) handleClientMessage(data []byte) error {
	var push protocol.Push
	err := push.Unmarshal(data)
	if err != nil {
		return err
	}
	switch push.Type {
	case protocol.PushTypePublication:
		var pub protocol.Publication
		err := pub.Unmarshal(push.Data)
		if err != nil {
			return err
		}
		_ = b.eventHandler.HandlePublication(push.Channel, &pub)
	case protocol.PushTypeJoin:
		var join protocol.Join
		err := join.Unmarshal(push.Data)
		if err != nil {
			return err
		}
		_ = b.eventHandler.HandleJoin(push.Channel, &join)
	case protocol.PushTypeLeave:
		var leave protocol.Leave
		err := leave.Unmarshal(push.Data)
		if err != nil {
			return err
		}
		_ = b.eventHandler.HandleLeave(push.Channel, &leave)
	default:
	}
	return nil
}

func (b *NatsBroker) handleClient(m *nats.Msg) {
	_ = b.handleClientMessage(m.Data)
}

func (b *NatsBroker) handleControl(m *nats.Msg) {
	_ = b.eventHandler.HandleControl(m.Data)
}

// Subscribe - see Broker interface description.
func (b *NatsBroker) Subscribe(ch string) error {
	if strings.Contains(ch, "*") || strings.Contains(ch, ">") {
		// Do not support wildcard subscriptions.
		return centrifuge.ErrorBadRequest
	}
	b.subsMu.Lock()
	defer b.subsMu.Unlock()
	clientChannel := b.clientChannel(ch)
	if _, ok := b.subs[clientChannel]; ok {
		return nil
	}
	subClient, err := b.nc.Subscribe(string(b.clientChannel(ch)), b.handleClient)
	if err != nil {
		return err
	}
	b.subs[clientChannel] = subClient
	return nil
}

// Unsubscribe - see Broker interface description.
func (b *NatsBroker) Unsubscribe(ch string) error {
	b.subsMu.Lock()
	defer b.subsMu.Unlock()
	if sub, ok := b.subs[b.clientChannel(ch)]; ok {
		_ = sub.Unsubscribe()
		delete(b.subs, b.clientChannel(ch))
	}
	return nil
}

// Channels - see Broker interface description.
func (b *NatsBroker) Channels() ([]string, error) {
	return nil, nil
}
