// Package pulsarbroker defines custom Nats Broker for Centrifuge library.
package pulsarbroker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/apache/pulsar-client-go/pulsar/log"
	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/sirupsen/logrus"
)

type (
	// channelID is unique channel identifier in Nats.
	channelID string
)

// Config of PulsarBroker.
type Config struct {
	URL               string
	Prefix            string
	Tenant            string
	Namespace         string
	OperationTimeout  time.Duration
	ConnectionTimeout time.Duration
}

// PulsarBroker is a broker on top of Nats messaging system.
type PulsarBroker struct {
	node   *centrifuge.Node
	config Config

	pc           pulsar.Client
	subsMu       sync.Mutex
	subs         map[channelID]pulsar.Consumer
	eventHandler centrifuge.BrokerEventHandler
	ctx          context.Context
	cancelFuncs  map[channelID]context.CancelFunc
}

var _ centrifuge.Broker = (*PulsarBroker)(nil)

// New creates PulsarBroker.
func New(n *centrifuge.Node, conf Config) (*PulsarBroker, error) {
	b := &PulsarBroker{
		node:        n,
		config:      conf,
		subs:        make(map[channelID]pulsar.Consumer),
		ctx:         context.Background(),
		cancelFuncs: make(map[channelID]context.CancelFunc),
	}
	return b, nil
}

func (b *PulsarBroker) controlChannel() channelID {
	return channelID(b.config.Prefix + ".control")
}

func (b *PulsarBroker) nodeChannel(nodeID string) channelID {
	return channelID(b.config.Prefix + ".node." + nodeID)
}

func (b *PulsarBroker) clientChannel(ch string) channelID {
	return channelID(b.config.Prefix + ".client." + ch)
}

// CustomWriter is an io.Writer that forwards logs to centrifugo.
type CustomWriter struct {
	node *centrifuge.Node
}

// filter out all other messages, otherwise there is a lot of noise in logs
func (cw *CustomWriter) Write(p []byte) (n int, err error) {
	whitelistMessages := []string{
		"Pulsar Broker connected to",
	}

	message := fmt.Sprintf("%v", string(p))

	for _, ignoreMsg := range whitelistMessages {
		if message != ignoreMsg {
			return
		}
	}

	cw.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, string(p), nil))
	return len(p), nil
}

// Run runs engine after node initialized.
func (b *PulsarBroker) Run(h centrifuge.BrokerEventHandler) error {
	b.eventHandler = h
	url := b.config.URL

	if url == "" {
		url = "pulsar://localhost:6650"
	}

	// Create a custom writer that forwards logs to centrifugo.
	customWriter := &CustomWriter{node: b.node}

	// Setup logger
	logrusLogger := logrus.New()
	logrusLogger.SetOutput(customWriter)

	pc, err := pulsar.NewClient(pulsar.ClientOptions{
		URL:               url,
		OperationTimeout:  b.config.OperationTimeout,
		ConnectionTimeout: b.config.ConnectionTimeout,
		Logger:            log.NewLoggerWithLogrus(logrusLogger),
	})

	if err != nil {
		return fmt.Errorf("error connecting to %s: %w", url, err)
	}

	// Shared channel for all subscriptions
	sharedChannel := make(chan pulsar.ConsumerMessage, 100)

	// Subscribe to control channel
	topic := b.genTopicString(string(b.controlChannel()))
	_, err = pc.Subscribe(pulsar.ConsumerOptions{
		Topic:                      topic,
		SubscriptionName:           "centrifugo.control",
		Type:                       pulsar.Exclusive,
		ReplicateSubscriptionState: true,
		MessageChannel:             sharedChannel,
	})

	if err != nil {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "could not create Pulsar control subscriber", map[string]any{"error": err.Error()}))
	}

	// Subscribe to node channel
	topic2 := b.genTopicString(string(b.nodeChannel(b.node.ID())))
	_, err = pc.Subscribe(pulsar.ConsumerOptions{
		Topic:                      topic2,
		SubscriptionName:           fmt.Sprintf("centrifugo.node.%s", b.node.ID()),
		Type:                       pulsar.Exclusive,
		ReplicateSubscriptionState: true,
		MessageChannel:             sharedChannel,
	})

	if err != nil {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, fmt.Sprintf("could not create Pulsar centrifugo.node.%s subscriber", b.node.ID()), map[string]any{"error": err.Error()}))
	}

	// Shared goroutine to handle messages from all subscriptions
	go func() {
		for cm := range sharedChannel {
			msg := cm.Message

			// Determine the type of message and handle accordingly
			if strings.Contains(msg.Topic(), string(b.controlChannel())) {
				b.handleControl(msg)
			} else if strings.Contains(msg.Topic(), string(b.nodeChannel(b.node.ID()))) {
				b.handleControl(msg) // Assuming handleControl is appropriate for node messages as well
			} else {
				b.handleClient(msg)
			}

			// Acknowledge the message
			cm.Consumer.Ack(msg)
		}
	}()

	b.pc = pc
	b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, fmt.Sprintf("Pulsar Broker connected to: %s", url)))
	return nil
}

// Close is not implemented.
func (b *PulsarBroker) Close(_ context.Context) error {
	return nil
}

// // Publish - see Broker interface description.
func (b *PulsarBroker) Publish(ch string, data []byte, opts centrifuge.PublishOptions) (centrifuge.StreamPosition, bool, error) {
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

	// Create a producer asynchronously
	topic := b.genTopicString(string(b.clientChannel(ch)))
	producer, err := b.pc.CreateProducer(pulsar.ProducerOptions{
		Topic: topic,
	})

	if err != nil {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "could not create Pulsar producer", map[string]any{"error": err.Error()}))
	}

	defer producer.Close()

	_, err = producer.Send(context.Background(), &pulsar.ProducerMessage{
		Payload: byteMessage,
	})

	return centrifuge.StreamPosition{}, false, err
}

// We experimentally support attaching epoch to publication.
const epochTagsKey = "__centrifugo_epoch"

func (b *PulsarBroker) PublishWithStreamPosition(ch string, data []byte, opts centrifuge.PublishOptions, sp centrifuge.StreamPosition) error {
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

	// Create a producer asynchronously
	topic := string(b.clientChannel(ch))
	producer, err := b.pc.CreateProducer(pulsar.ProducerOptions{
		Topic: topic,
	})

	if err != nil {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "could not create Pulsar producer", map[string]any{"error": err.Error()}))
	}

	defer producer.Close()

	_, err = producer.Send(context.Background(), &pulsar.ProducerMessage{
		Payload: byteMessage,
	})

	return err
}

// PublishJoin - see Broker interface description.
func (b *PulsarBroker) PublishJoin(ch string, info *centrifuge.ClientInfo) error {
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

	// Create a producer asynchronously
	topic := string(b.clientChannel(ch))
	producer, err := b.pc.CreateProducer(pulsar.ProducerOptions{
		Topic: topic,
	})

	if err != nil {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "could not create Pulsar producer", map[string]any{"error": err.Error()}))
	}

	defer producer.Close()

	_, err = producer.Send(context.Background(), &pulsar.ProducerMessage{
		Payload: byteMessage,
	})

	return err
}

// PublishLeave - see Broker interface description.
func (b *PulsarBroker) PublishLeave(ch string, info *centrifuge.ClientInfo) error {
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

	// Create a producer asynchronously
	topic := string(b.clientChannel(ch))
	producer, err := b.pc.CreateProducer(pulsar.ProducerOptions{
		Topic: topic,
	})

	if err != nil {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "could not create Pulsar producer", map[string]any{"error": err.Error()}))
	}

	defer producer.Close()

	_, err = producer.Send(context.Background(), &pulsar.ProducerMessage{
		Payload: byteMessage,
	})

	return err
}

// PublishControl - see Broker interface description.
func (b *PulsarBroker) PublishControl(data []byte, nodeID, _ string) error {
	var channelID channelID
	if nodeID == "" {
		channelID = b.controlChannel()
	} else {
		channelID = b.nodeChannel(nodeID)
	}

	// Create a producer asynchronously
	topic := string(string(channelID))
	producer, err := b.pc.CreateProducer(pulsar.ProducerOptions{
		Topic: topic,
		Name:  "producer-control",
	})

	if err != nil {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "could not create Pulsar producer", map[string]any{"error": err.Error()}))
	}

	defer producer.Close()

	_, err = producer.Send(context.Background(), &pulsar.ProducerMessage{
		Payload: data,
	})

	return err
}

// History ...
func (b *PulsarBroker) History(_ string, _ centrifuge.HistoryOptions) ([]*centrifuge.Publication, centrifuge.StreamPosition, error) {
	return nil, centrifuge.StreamPosition{}, centrifuge.ErrorNotAvailable
}

// RemoveHistory ...
func (b *PulsarBroker) RemoveHistory(_ string) error {
	return centrifuge.ErrorNotAvailable
}

func (b *PulsarBroker) handleClientMessage(data []byte) {
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

func (b *PulsarBroker) handleClient(m pulsar.Message) {
	b.handleClientMessage(m.Payload())
}

func (b *PulsarBroker) handleControl(m pulsar.Message) {
	_ = b.eventHandler.HandleControl(m.Payload())
}

// Subscribe - see Broker interface description.
func (b *PulsarBroker) Subscribe(ch string) error {
	b.subsMu.Lock()
	defer b.subsMu.Unlock()
	clientChannel := b.clientChannel(ch)
	if _, ok := b.subs[clientChannel]; ok {
		return nil
	}

	topic := b.genTopicString(string(b.clientChannel(ch)))
	channel := make(chan pulsar.ConsumerMessage, 100)

	consumer, err := b.pc.Subscribe(pulsar.ConsumerOptions{
		Topic:                      topic,
		SubscriptionName:           "subscriber",
		Type:                       pulsar.Exclusive,
		ReplicateSubscriptionState: true,
		MessageChannel:             channel,
	})

	if err != nil {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "could not create Pulsar subscriber", map[string]any{"error": err.Error()}))
	}

	ctx, cancel := context.WithCancel(b.ctx)

	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				// Context was canceled, exit the goroutine
				return
			case cm := <-channel:
				msg := cm.Message
				b.handleClient(msg)
				consumer.Ack(msg)
			}
		}
	}(ctx)

	b.subs[clientChannel] = consumer
	b.cancelFuncs[clientChannel] = cancel

	return nil
}

// Unsubscribe - see Broker interface description.
func (b *PulsarBroker) Unsubscribe(ch string) error {
	b.subsMu.Lock()
	defer b.subsMu.Unlock()
	if sub, ok := b.subs[b.clientChannel(ch)]; ok {
		_ = sub.Unsubscribe()
		delete(b.subs, b.clientChannel(ch))
	}
	if cancel, ok := b.cancelFuncs[b.clientChannel(ch)]; ok {
		cancel()
		delete(b.cancelFuncs, b.clientChannel(ch))
	}
	return nil
}

// Channels - see Broker interface description.
func (b *PulsarBroker) Channels() ([]string, error) {
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

func (b *PulsarBroker) genTopicString(topic string) string {
	// generate topic string
	connectionArr := []string{b.config.Tenant, b.config.Namespace, topic}

	return strings.Join(connectionArr, "/")
}
