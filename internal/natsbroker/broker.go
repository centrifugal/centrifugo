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

func IsUnsupportedChannel(ch string) bool {
	return strings.Contains(ch, "*") || strings.Contains(ch, ">")
}

// Publish - see Broker interface description.
func (b *NatsBroker) Publish(ch string, data []byte, opts centrifuge.PublishOptions) (centrifuge.StreamPosition, bool, error) {
	if IsUnsupportedChannel(ch) {
		// Do not support wildcard subscriptions.
		return centrifuge.StreamPosition{}, false, centrifuge.ErrorBadRequest
	}
	push := &protocol.Push{
		Channel: ch,
		Pub: &protocol.Publication{
			Data: data,
			Info: infoToProto(opts.ClientInfo),
			Tags: opts.Tags,
		},
	}
	byteMessage, err := push.MarshalVT()
	if err != nil {
		return centrifuge.StreamPosition{}, false, err
	}
	return centrifuge.StreamPosition{}, false, b.nc.Publish(string(b.clientChannel(ch)), byteMessage)
}

// We experimentally support attaching epoch to publication.
const epochTagsKey = "__centrifugo_epoch"

func (b *NatsBroker) PublishWithStreamPosition(ch string, data []byte, opts centrifuge.PublishOptions, sp centrifuge.StreamPosition) error {
	if IsUnsupportedChannel(ch) {
		// Do not support wildcard subscriptions.
		return centrifuge.ErrorBadRequest
	}
	tags := opts.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	tags[epochTagsKey] = sp.Epoch
	push := &protocol.Push{
		Channel: ch,
		Pub: &protocol.Publication{
			Offset: sp.Offset,
			Data:   data,
			Info:   infoToProto(opts.ClientInfo),
			Tags:   tags,
		},
	}
	byteMessage, err := push.MarshalVT()
	if err != nil {
		return err
	}
	return b.nc.Publish(string(b.clientChannel(ch)), byteMessage)
}

// PublishJoin - see Broker interface description.
func (b *NatsBroker) PublishJoin(ch string, info *centrifuge.ClientInfo) error {
	push := &protocol.Push{
		Channel: ch,
		Join: &protocol.Join{
			Info: infoToProto(info),
		},
	}
	byteMessage, err := push.MarshalVT()
	if err != nil {
		return err
	}
	return b.nc.Publish(string(b.clientChannel(ch)), byteMessage)
}

// PublishLeave - see Broker interface description.
func (b *NatsBroker) PublishLeave(ch string, info *centrifuge.ClientInfo) error {
	push := &protocol.Push{
		Channel: ch,
		Leave: &protocol.Leave{
			Info: infoToProto(info),
		},
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
func (b *NatsBroker) History(_ string, _ centrifuge.HistoryOptions) ([]*centrifuge.Publication, centrifuge.StreamPosition, error) {
	return nil, centrifuge.StreamPosition{}, centrifuge.ErrorNotAvailable
}

// RemoveHistory ...
func (b *NatsBroker) RemoveHistory(_ string) error {
	return centrifuge.ErrorNotAvailable
}

func (b *NatsBroker) handleClientMessage(data []byte) {
	var push protocol.Push
	err := push.UnmarshalVT(data)
	if err != nil {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "can't unmarshal push from Nats", map[string]any{"error": err.Error()}))
		return
	}
	if push.Pub != nil {
		sp := centrifuge.StreamPosition{}
		if push.Pub.Offset > 0 && push.Pub.Tags != nil {
			sp.Offset = push.Pub.Offset
			sp.Epoch = push.Pub.Tags[epochTagsKey]
		}
		_ = b.eventHandler.HandlePublication(push.Channel, pubFromProto(push.Pub), sp)
	} else if push.Join != nil {
		_ = b.eventHandler.HandleJoin(push.Channel, infoFromProto(push.Join.Info))
	} else if push.Leave != nil {
		_ = b.eventHandler.HandleLeave(push.Channel, infoFromProto(push.Leave.Info))
	} else {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "unknown push from Nats", map[string]any{"push": fmt.Sprintf("%v", &push)}))
	}
}

func (b *NatsBroker) handleClient(m *nats.Msg) {
	b.handleClientMessage(m.Data)
}

func (b *NatsBroker) handleControl(m *nats.Msg) {
	_ = b.eventHandler.HandleControl(m.Data)
}

// Subscribe - see Broker interface description.
func (b *NatsBroker) Subscribe(ch string) error {
	if IsUnsupportedChannel(ch) {
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
