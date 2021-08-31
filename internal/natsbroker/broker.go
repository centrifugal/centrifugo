// Package natsbroker defines custom Nats Broker for Centrifuge library.
package natsbroker

import (
	"context"
	"fmt"
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

var _ centrifuge.Broker = (*NatsBroker)(nil)

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

func (b *NatsBroker) nodeChannel(nodeID string) channelID {
	return channelID(b.config.Prefix + ".node." + nodeID)
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
		nats.MaxReconnects(-1),
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
	_, err = nc.Subscribe(string(b.nodeChannel(b.node.ID())), b.handleControl)
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

func isUnsupportedChannel(ch string) bool {
	return strings.Contains(ch, "*") || strings.Contains(ch, ">")
}

// Publish - see Broker interface description.
func (b *NatsBroker) Publish(ch string, data []byte, opts centrifuge.PublishOptions) (centrifuge.StreamPosition, error) {
	if isUnsupportedChannel(ch) {
		// Do not support wildcard subscriptions.
		return centrifuge.StreamPosition{}, centrifuge.ErrorBadRequest
	}
	protoPub := &protocol.Publication{
		Data: data,
		Info: infoToProto(opts.ClientInfo),
	}
	data, err := protoPub.MarshalVT()
	if err != nil {
		return centrifuge.StreamPosition{}, err
	}
	push := &protocol.Push{
		Type:    protocol.Push_PUBLICATION,
		Channel: ch,
		Data:    data,
	}
	byteMessage, err := push.MarshalVT()
	if err != nil {
		return centrifuge.StreamPosition{}, err
	}
	return centrifuge.StreamPosition{}, b.nc.Publish(string(b.clientChannel(ch)), byteMessage)
}

// PublishJoin - see Broker interface description.
func (b *NatsBroker) PublishJoin(ch string, info *centrifuge.ClientInfo) error {
	data, err := infoToProto(info).MarshalVT()
	if err != nil {
		return err
	}
	push := &protocol.Push{
		Type:    protocol.Push_JOIN,
		Channel: ch,
		Data:    data,
	}
	byteMessage, err := push.MarshalVT()
	if err != nil {
		return err
	}
	return b.nc.Publish(string(b.clientChannel(ch)), byteMessage)
}

// PublishLeave - see Broker interface description.
func (b *NatsBroker) PublishLeave(ch string, info *centrifuge.ClientInfo) error {
	data, err := infoToProto(info).MarshalVT()
	if err != nil {
		return err
	}
	push := &protocol.Push{
		Type:    protocol.Push_LEAVE,
		Channel: ch,
		Data:    data,
	}
	byteMessage, err := push.MarshalVT()
	if err != nil {
		return err
	}
	return b.nc.Publish(string(b.clientChannel(ch)), byteMessage)
}

// PublishControl - see Broker interface description.
func (b *NatsBroker) PublishControl(data []byte, nodeID, _ string) error {
	var channelID channelID
	if nodeID == "" {
		channelID = b.controlChannel()
	} else {
		channelID = b.nodeChannel(nodeID)
	}
	return b.nc.Publish(string(channelID), data)
}

// History ...
func (b *NatsBroker) History(_ string, _ centrifuge.HistoryFilter) ([]*centrifuge.Publication, centrifuge.StreamPosition, error) {
	return nil, centrifuge.StreamPosition{}, centrifuge.ErrorNotAvailable
}

// RemoveHistory ...
func (b *NatsBroker) RemoveHistory(_ string) error {
	return centrifuge.ErrorNotAvailable
}

func (b *NatsBroker) handleClientMessage(data []byte) error {
	var push protocol.Push
	err := push.UnmarshalVT(data)
	if err != nil {
		return err
	}
	switch push.Type {
	case protocol.Push_PUBLICATION:
		var pub protocol.Publication
		err := pub.UnmarshalVT(push.Data)
		if err != nil {
			return err
		}
		_ = b.eventHandler.HandlePublication(push.Channel, pubFromProto(&pub), centrifuge.StreamPosition{})
	case protocol.Push_JOIN:
		var info protocol.ClientInfo
		err := info.UnmarshalVT(push.Data)
		if err != nil {
			return err
		}
		_ = b.eventHandler.HandleJoin(push.Channel, infoFromProto(&info))
	case protocol.Push_LEAVE:
		var info protocol.ClientInfo
		err := info.UnmarshalVT(push.Data)
		if err != nil {
			return err
		}
		_ = b.eventHandler.HandleLeave(push.Channel, infoFromProto(&info))
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
	if isUnsupportedChannel(ch) {
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

func infoFromProto(v *protocol.ClientInfo) *centrifuge.ClientInfo {
	if v == nil {
		return nil
	}
	info := &centrifuge.ClientInfo{
		ClientID: v.GetClient(),
		UserID:   v.GetUser(),
	}
	if len(v.ConnInfo) > 0 {
		info.ConnInfo = v.ConnInfo
	}
	if len(v.ChanInfo) > 0 {
		info.ChanInfo = v.ChanInfo
	}
	return info
}

func infoToProto(v *centrifuge.ClientInfo) *protocol.ClientInfo {
	if v == nil {
		return nil
	}
	info := &protocol.ClientInfo{
		Client: v.ClientID,
		User:   v.UserID,
	}
	if len(v.ConnInfo) > 0 {
		info.ConnInfo = v.ConnInfo
	}
	if len(v.ChanInfo) > 0 {
		info.ChanInfo = v.ChanInfo
	}
	return info
}

func pubFromProto(pub *protocol.Publication) *centrifuge.Publication {
	if pub == nil {
		return nil
	}
	return &centrifuge.Publication{
		Offset: pub.GetOffset(),
		Data:   pub.Data,
		Info:   infoFromProto(pub.GetInfo()),
	}
}
