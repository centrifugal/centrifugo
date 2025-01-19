// Package natsbroker defines custom Nats Broker for Centrifuge library.
package natsbroker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

type (
	// channelID is unique channel identifier in Nats.
	channelID string
)

type Config = configtypes.NatsBroker

type subWrapper struct {
	sub         *nats.Subscription
	origChannel string
}

// NatsBroker is a broker on top of Nats messaging system.
type NatsBroker struct {
	node   *centrifuge.Node
	config Config

	nc                  *nats.Conn
	subsMu              sync.RWMutex
	subs                map[channelID]subWrapper
	eventHandler        centrifuge.BrokerEventHandler
	clientChannelPrefix string
	rawModeReplacer     *strings.Replacer
}

var _ centrifuge.Broker = (*NatsBroker)(nil)

// New creates NatsBroker.
func New(n *centrifuge.Node, conf Config) (*NatsBroker, error) {
	b := &NatsBroker{
		node:                n,
		config:              conf,
		subs:                make(map[channelID]subWrapper),
		clientChannelPrefix: conf.Prefix + ".client.",
	}
	if conf.RawMode.Enabled {
		log.Info().Str("raw_mode_prefix", conf.RawMode.Prefix).Msg("raw mode of Nats enabled")
		if len(conf.RawMode.ChannelReplacements) > 0 {
			var replacerArgs []string
			for k, v := range conf.RawMode.ChannelReplacements {
				replacerArgs = append(replacerArgs, k, v)
			}
			b.rawModeReplacer = strings.NewReplacer(replacerArgs...)
		}
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
	if b.config.RawMode.Enabled {
		if b.rawModeReplacer != nil {
			ch = b.rawModeReplacer.Replace(ch)
		}
		return channelID(b.config.RawMode.Prefix + ch)
	}
	return channelID(b.clientChannelPrefix + ch)
}

// Run runs engine after node initialized.
func (b *NatsBroker) Run(h centrifuge.BrokerEventHandler) error {
	b.eventHandler = h
	url := b.config.URL
	if url == "" {
		url = nats.DefaultURL
	}
	options := []nats.Option{
		nats.ReconnectBufSize(-1),
		nats.MaxReconnects(-1),
		nats.Timeout(b.config.DialTimeout.ToDuration()),
		nats.FlusherTimeout(b.config.WriteTimeout.ToDuration()),
	}
	if b.config.TLS.Enabled {
		tlsConfig, err := b.config.TLS.ToGoTLSConfig("nats")
		if err != nil {
			return fmt.Errorf("error creating TLS config: %w", err)
		}
		options = append(options, nats.Secure(tlsConfig))
	}
	nc, err := nats.Connect(url, options...)
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
	log.Info().Str("broker", "nats").Str("url", url).Msg("broker running")
	return nil
}

// Close ...
func (b *NatsBroker) Close(_ context.Context) error {
	b.nc.Close()
	return nil
}

func (b *NatsBroker) IsSupportedSubscribeChannel(ch string) bool {
	if b.config.AllowWildcards {
		return true
	}
	if strings.Contains(ch, "*") || strings.Contains(ch, ">") {
		return false
	}
	return true
}

func (b *NatsBroker) IsSupportedPublishChannel(ch string) bool {
	if strings.Contains(ch, "*") || strings.Contains(ch, ">") {
		return false
	}
	return true
}

// Publish - see Broker interface description.
func (b *NatsBroker) Publish(ch string, data []byte, opts centrifuge.PublishOptions) (centrifuge.StreamPosition, bool, error) {
	if !b.IsSupportedPublishChannel(ch) {
		// Do not support wildcard subscriptions.
		return centrifuge.StreamPosition{}, false, centrifuge.ErrorBadRequest
	}
	if b.config.RawMode.Enabled {
		return centrifuge.StreamPosition{}, false, b.nc.Publish(b.config.RawMode.Prefix+ch, data)
	}
	push := &protocol.Push{
		Channel: ch,
		Pub: &protocol.Publication{
			Data:  data,
			Info:  infoToProto(opts.ClientInfo),
			Tags:  opts.Tags,
			Delta: opts.UseDelta, // Will be cleaned up before passing to Node.
			Time:  time.Now().UnixMilli(),
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
	if !b.IsSupportedPublishChannel(ch) {
		// Do not support wildcard subscriptions.
		return centrifuge.ErrorBadRequest
	}
	if b.config.RawMode.Enabled {
		// Do not support stream positions in raw mode.
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
			Delta:  opts.UseDelta, // Will be cleaned up before passing to Node.
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
	if b.config.RawMode.Enabled {
		return nil
	}
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
	if b.config.RawMode.Enabled {
		return nil
	}
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

func (b *NatsBroker) handleClientMessage(subject string, data []byte, sub *nats.Subscription) {
	if b.config.RawMode.Enabled {
		b.subsMu.RLock()
		subWrap, ok := b.subs[channelID(sub.Subject)]
		b.subsMu.RUnlock()
		if !ok {
			return
		}
		channel := subWrap.origChannel
		_ = b.eventHandler.HandlePublication(
			channel,
			&centrifuge.Publication{Data: data, Channel: strings.TrimPrefix(subject, b.config.RawMode.Prefix)},
			centrifuge.StreamPosition{}, false, nil)
		return
	}

	var push protocol.Push
	err := push.UnmarshalVT(data)
	if err != nil {
		log.Error().Err(err).Msg("error unmarshal push from Nats")
		return
	}

	if push.Pub != nil {
		var subChannel = push.Channel
		var specificChannel string
		if b.config.AllowWildcards {
			subChannel = strings.TrimPrefix(sub.Subject, b.clientChannelPrefix)
			specificChannel = push.Channel
		}
		sp := centrifuge.StreamPosition{}
		if push.Pub.Offset > 0 && push.Pub.Tags != nil {
			sp.Offset = push.Pub.Offset
			sp.Epoch = push.Pub.Tags[epochTagsKey]
		}
		delta := push.Pub.Delta
		push.Pub.Delta = false
		_ = b.eventHandler.HandlePublication(subChannel, pubFromProto(push.Pub, specificChannel), sp, delta, nil)
	} else if push.Join != nil {
		_ = b.eventHandler.HandleJoin(push.Channel, infoFromProto(push.Join.Info))
	} else if push.Leave != nil {
		_ = b.eventHandler.HandleLeave(push.Channel, infoFromProto(push.Leave.Info))
	} else {
		log.Warn().Str("push", fmt.Sprintf("%v", &push)).Msg("unknown push from Nats")
	}
}

func (b *NatsBroker) handleClient(m *nats.Msg) {
	b.handleClientMessage(m.Subject, m.Data, m.Sub)
}

func (b *NatsBroker) handleControl(m *nats.Msg) {
	_ = b.eventHandler.HandleControl(m.Data)
}

// Subscribe - see Broker interface description.
func (b *NatsBroker) Subscribe(ch string) error {
	if !b.IsSupportedSubscribeChannel(ch) {
		// Do not support wildcard subscriptions.
		return centrifuge.ErrorBadRequest
	}
	clientChannel := b.clientChannel(ch)
	b.subsMu.RLock()
	_, ok := b.subs[clientChannel]
	b.subsMu.RUnlock()
	if ok {
		return nil
	}
	subscription, err := b.nc.Subscribe(string(clientChannel), b.handleClient)
	if err != nil {
		return err
	}
	b.subsMu.Lock()
	defer b.subsMu.Unlock()
	b.subs[clientChannel] = subWrapper{
		sub:         subscription,
		origChannel: ch,
	}
	return nil
}

// Unsubscribe - see Broker interface description.
func (b *NatsBroker) Unsubscribe(ch string) error {
	clientChannel := b.clientChannel(ch)
	b.subsMu.RLock()
	subWrap, ok := b.subs[clientChannel]
	b.subsMu.RUnlock()
	if ok {
		err := subWrap.sub.Unsubscribe()
		if err != nil {
			return err
		}
		b.subsMu.Lock()
		defer b.subsMu.Unlock()
		delete(b.subs, clientChannel)
	}
	return nil
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

func pubFromProto(pub *protocol.Publication, specificChannel string) *centrifuge.Publication {
	if pub == nil {
		return nil
	}
	return &centrifuge.Publication{
		Offset:  pub.GetOffset(),
		Data:    pub.Data,
		Info:    infoFromProto(pub.GetInfo()),
		Channel: specificChannel,
	}
}
