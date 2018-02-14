// Package node is a real-time core for Centrifugo server.
package node

import (
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/lib/channel"
	"github.com/centrifugal/centrifugo/lib/conns"
	"github.com/centrifugal/centrifugo/lib/engine"
	"github.com/centrifugal/centrifugo/lib/events"
	"github.com/centrifugal/centrifugo/lib/logging"
	"github.com/centrifugal/centrifugo/lib/proto"
	"github.com/centrifugal/centrifugo/lib/proto/apiproto"
	"github.com/centrifugal/centrifugo/lib/proto/controlproto"

	"github.com/nats-io/nuid"
	"github.com/satori/go.uuid"
)

// Node is a heart of Centrifugo – it internally keeps and manages client
// connections, maintains information about other Centrifugo nodes, keeps
// some useful references to things like engine, metrics etc.
type Node struct {
	mu sync.RWMutex

	// version of Centrifugo node.
	version string

	// unique id for this node.
	uid string

	// startedAt is unix time of node start.
	startedAt int64

	// config for node.
	config *Config

	// hub to manage client connections.
	hub conns.Hub

	// engine - in memory or redis.
	engine engine.Engine

	// nodes contains registry of known nodes.
	nodes *nodeRegistry

	// shutdown is a flag which is only true when node is going to shut down.
	shutdown bool

	// shutdownCh is a channel which is closed when node shutdown initiated.
	shutdownCh chan struct{}

	// messageEncoder is encoder to encode messages for engine.
	messageEncoder proto.MessageEncoder

	// messageEncoder is decoder to decode messages coming from engine.
	messageDecoder proto.MessageDecoder

	// controlEncoder is encoder to encode control messages for engine.
	controlEncoder controlproto.Encoder

	// controlDecoder is decoder to decode control messages coming from engine.
	controlDecoder controlproto.Decoder

	// mediator contains application event handlers.
	mediator *events.Mediator

	logger logging.Logger
}

// VERSION of Centrifugo server node. Set on build stage.
var VERSION string

// New creates Node, the only required argument is config.
func New(c *Config) *Node {
	uid := uuid.NewV4().String()

	n := &Node{
		version:        VERSION,
		uid:            uid,
		nodes:          newNodeRegistry(uid),
		config:         c,
		hub:            conns.NewHub(),
		startedAt:      time.Now().Unix(),
		shutdownCh:     make(chan struct{}),
		messageEncoder: proto.NewProtobufMessageEncoder(),
		messageDecoder: proto.NewProtobufMessageDecoder(),
		controlEncoder: controlproto.NewProtobufEncoder(),
		controlDecoder: controlproto.NewProtobufDecoder(),
		logger:         logging.New(logging.NONE, nil),
	}
	return n
}

// SetLogger sets Logger to Node. It's not concurrency safe and should
// only be called once at application start.
func (n *Node) SetLogger(l logging.Logger) {
	n.logger = l
}

// Logger returns registered Logger. If no logger registered we return
// nil HandlerLogger which does not log anything.
func (n *Node) Logger() logging.Logger {
	return n.logger
}

// Config returns a copy of node Config.
func (n *Node) Config() Config {
	n.mu.RLock()
	c := *n.config
	n.mu.RUnlock()
	return c
}

// SetConfig binds config to node.
func (n *Node) SetConfig(c *Config) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.config = c
}

// SetMediator binds mediator to node.
func (n *Node) SetMediator(m *events.Mediator) {
	n.mediator = m
}

// Mediator binds config to node.
func (n *Node) Mediator() *events.Mediator {
	return n.mediator
}

// Version returns version of node.
func (n *Node) Version() string {
	return n.version
}

// Reload node.
func (n *Node) Reload(c *Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	n.SetConfig(c)
	return nil
}

// Engine returns node's Engine.
func (n *Node) Engine() engine.Engine {
	return n.engine
}

// Hub returns node's client hub.
func (n *Node) Hub() conns.Hub {
	return n.hub
}

// MessageEncoder ...
func (n *Node) MessageEncoder() proto.MessageEncoder {
	return n.messageEncoder
}

// MessageDecoder ...
func (n *Node) MessageDecoder() proto.MessageDecoder {
	return n.messageDecoder
}

// ControlEncoder ...
func (n *Node) ControlEncoder() controlproto.Encoder {
	return n.controlEncoder
}

// ControlDecoder ...
func (n *Node) ControlDecoder() controlproto.Decoder {
	return n.controlDecoder
}

// NotifyShutdown returns a channel which will be closed on node shutdown.
func (n *Node) NotifyShutdown() chan struct{} {
	return n.shutdownCh
}

// Run performs all startup actions. At moment must be called once on start
// after engine and structure set.
func (n *Node) Run(e engine.Engine) error {
	n.mu.Lock()
	n.engine = e
	n.mu.Unlock()

	if err := n.engine.Run(); err != nil {
		return err
	}

	err := n.pubNode()
	if err != nil {
		n.Logger().Log(logging.NewEntry(logging.ERROR, "error publishing node control command", map[string]interface{}{"error": err.Error()}))
	}
	go n.sendNodePingMsg()
	go n.cleanNodeInfo()
	go n.updateMetrics()

	return nil
}

// Shutdown sets shutdown flag and does various clean ups.
func (n *Node) Shutdown() error {
	n.mu.Lock()
	if n.shutdown {
		n.mu.Unlock()
		return nil
	}
	n.shutdown = true
	close(n.shutdownCh)
	n.mu.Unlock()
	return n.hub.Shutdown()
}

func (n *Node) updateMetricsOnce() {
	numClientsGauge.Set(float64(n.hub.NumClients()))
	numUsersGauge.Set(float64(n.hub.NumUsers()))
	numChannelsGauge.Set(float64(n.hub.NumChannels()))
}

func (n *Node) updateMetrics() {
	for {
		n.mu.RLock()
		interval := n.config.NodeMetricsInterval
		n.mu.RUnlock()
		select {
		case <-n.shutdownCh:
			return
		case <-time.After(interval):
			n.updateMetricsOnce()
		}
	}
}

func (n *Node) sendNodePingMsg() {
	for {
		n.mu.RLock()
		interval := n.config.NodePingInterval
		n.mu.RUnlock()

		select {
		case <-n.shutdownCh:
			return
		case <-time.After(interval):
			err := n.pubNode()
			if err != nil {
				n.Logger().Log(logging.NewEntry(logging.ERROR, "error publishing node control command", map[string]interface{}{"error": err.Error()}))
			}
		}
	}
}

func (n *Node) cleanNodeInfo() {
	for {
		n.mu.RLock()
		interval := n.config.NodeInfoCleanInterval
		n.mu.RUnlock()
		select {
		case <-n.shutdownCh:
			return
		case <-time.After(interval):
			n.mu.RLock()
			delay := n.config.NodeInfoMaxDelay
			n.mu.RUnlock()
			n.nodes.clean(delay)
		}
	}
}

// Channels returns list of all engines clients subscribed on all Centrifugo nodes.
func (n *Node) Channels() ([]string, error) {
	return n.engine.Channels()
}

// Info returns aggregated stats from all Centrifugo nodes.
func (n *Node) Info() (*apiproto.InfoResult, error) {
	nodes := n.nodes.list()
	nodeResults := make([]*apiproto.NodeResult, len(nodes))
	for i, nd := range nodes {
		nodeResults[i] = &apiproto.NodeResult{
			UID:         nd.UID,
			Name:        nd.Name,
			NumClients:  nd.NumClients,
			NumUsers:    nd.NumUsers,
			NumChannels: nd.NumChannels,
			Uptime:      nd.Uptime,
		}
	}

	return &apiproto.InfoResult{
		Engine: n.Engine().Name(),
		Nodes:  nodeResults,
	}, nil
}

// HandleControl handles messages from control channel - control messages used for internal
// communication between nodes to share state or proto.
func (n *Node) HandleControl(cmd *controlproto.Command) error {
	messagesReceivedCount.WithLabelValues("control").Inc()

	if cmd.UID == n.uid {
		// Sent by this node.
		return nil
	}

	method := cmd.Method
	params := cmd.Params

	switch method {
	case controlproto.MethodTypeNode:
		cmd, err := n.ControlDecoder().DecodeNode(params)
		if err != nil {
			n.Logger().Log(logging.NewEntry(logging.ERROR, "error decoding node control params", map[string]interface{}{"error": err.Error()}))
			return proto.ErrBadRequest
		}
		return n.nodeCmd(cmd)
	case controlproto.MethodTypeUnsubscribe:
		cmd, err := n.ControlDecoder().DecodeUnsubscribe(params)
		if err != nil {
			n.Logger().Log(logging.NewEntry(logging.ERROR, "error decoding unsubscribe control params", map[string]interface{}{"error": err.Error()}))
			return proto.ErrBadRequest
		}
		return n.Hub().Unsubscribe(cmd.User, cmd.Channel)
	case controlproto.MethodTypeDisconnect:
		cmd, err := n.ControlDecoder().DecodeDisconnect(params)
		if err != nil {
			n.Logger().Log(logging.NewEntry(logging.ERROR, "error decoding disconnect control params", map[string]interface{}{"error": err.Error()}))
			return proto.ErrBadRequest
		}
		return n.Hub().Disconnect(cmd.User, false)
	default:
		n.Logger().Log(logging.NewEntry(logging.ERROR, "unknown control message method", map[string]interface{}{"method": method}))
		return proto.ErrBadRequest
	}
}

// HandleClientMessage ...
func (n *Node) HandleClientMessage(message *proto.Message) error {
	switch message.Type {
	case proto.MessageTypePublication:
		publication, err := n.messageDecoder.DecodePublication(message.Data)
		if err != nil {
			return err
		}
		n.HandlePublication(message.Channel, publication)
	case proto.MessageTypeJoin:
		join, err := n.messageDecoder.DecodeJoin(message.Data)
		if err != nil {
			return err
		}
		n.HandleJoin(message.Channel, join)
	case proto.MessageTypeLeave:
		leave, err := n.messageDecoder.DecodeLeave(message.Data)
		if err != nil {
			return err
		}
		n.HandleLeave(message.Channel, leave)
	default:
	}
	return nil
}

// HandlePublication handles messages published by web application or client into channel.
// The goal of this method to deliver this message to all clients on this node subscribed
// on channel.
func (n *Node) HandlePublication(ch string, publication *proto.Publication) error {
	messagesReceivedCount.WithLabelValues("publication").Inc()
	numSubscribers := n.hub.NumSubscribers(ch)
	hasCurrentSubscribers := numSubscribers > 0
	if !hasCurrentSubscribers {
		return nil
	}
	return n.hub.BroadcastPublication(ch, publication)
}

// HandleJoin handles join messages.
func (n *Node) HandleJoin(ch string, join *proto.Join) error {
	messagesReceivedCount.WithLabelValues("join").Inc()
	hasCurrentSubscribers := n.hub.NumSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	return n.hub.BroadcastJoin(ch, join)
}

// HandleLeave handles leave messages.
func (n *Node) HandleLeave(ch string, leave *proto.Leave) error {
	messagesReceivedCount.WithLabelValues("leave").Inc()
	hasCurrentSubscribers := n.hub.NumSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	return n.hub.BroadcastLeave(ch, leave)
}

func makeErrChan(err error) <-chan error {
	ret := make(chan error, 1)
	ret <- err
	return ret
}

// Publish sends a message to all clients subscribed on channel. All running nodes
// will receive it and will send it to all clients on node subscribed on channel.
func (n *Node) Publish(ch string, pub *proto.Publication, opts *channel.Options) <-chan error {
	if opts == nil {
		chOpts, ok := n.ChannelOpts(ch)
		if !ok {
			return makeErrChan(proto.ErrNamespaceNotFound)
		}
		opts = &chOpts
	}

	messagesSentCount.WithLabelValues("publication").Inc()

	if pub.UID == "" {
		pub.UID = nuid.Next()
	}

	return n.engine.Publish(ch, pub, opts)
}

// PublishJoin allows to publish join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (n *Node) PublishJoin(ch string, join *proto.Join, opts *channel.Options) <-chan error {
	if opts == nil {
		chOpts, ok := n.ChannelOpts(ch)
		if !ok {
			return makeErrChan(proto.ErrNamespaceNotFound)
		}
		opts = &chOpts
	}
	messagesSentCount.WithLabelValues("join").Inc()
	return n.engine.PublishJoin(ch, join, opts)
}

// PublishLeave allows to publish join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (n *Node) PublishLeave(ch string, leave *proto.Leave, opts *channel.Options) <-chan error {
	if opts == nil {
		chOpts, ok := n.ChannelOpts(ch)
		if !ok {
			return makeErrChan(proto.ErrNamespaceNotFound)
		}
		opts = &chOpts
	}
	messagesSentCount.WithLabelValues("leave").Inc()
	return n.engine.PublishLeave(ch, leave, opts)
}

// publishControl publishes message into control channel so all running
// nodes will receive and handle it.
func (n *Node) publishControl(msg *controlproto.Command) <-chan error {
	messagesSentCount.WithLabelValues("control").Inc()
	return n.engine.PublishControl(msg)
}

// pubNode sends control message to all nodes - this message
// contains information about current node.
func (n *Node) pubNode() error {
	n.mu.RLock()
	node := &controlproto.Node{
		UID:         n.uid,
		Name:        n.config.Name,
		Version:     n.version,
		NumClients:  uint32(n.hub.NumClients()),
		NumUsers:    uint32(n.hub.NumUsers()),
		NumChannels: uint32(n.hub.NumChannels()),
		Uptime:      uint32(time.Now().Unix() - n.startedAt),
	}
	n.mu.RUnlock()

	params, _ := n.ControlEncoder().EncodeNode(node)

	cmd := &controlproto.Command{
		UID:    n.uid,
		Method: controlproto.MethodTypeNode,
		Params: params,
	}

	err := n.nodeCmd(node)
	if err != nil {
		n.Logger().Log(logging.NewEntry(logging.ERROR, "error handling node command", map[string]interface{}{"error": err.Error()}))
	}

	return <-n.publishControl(cmd)
}

// pubUnsubscribe publishes unsubscribe control message to all nodes – so all
// nodes could unsubscribe user from channel.
func (n *Node) pubUnsubscribe(user string, ch string) error {

	// TODO: looks it's already ok - need to check.
	unsubscribe := &controlproto.Unsubscribe{
		User:    user,
		Channel: ch,
	}

	params, _ := n.ControlEncoder().EncodeUnsubscribe(unsubscribe)

	cmd := &controlproto.Command{
		UID:    n.uid,
		Method: controlproto.MethodTypeUnsubscribe,
		Params: params,
	}

	return <-n.publishControl(cmd)
}

// pubDisconnect publishes disconnect control message to all nodes – so all
// nodes could disconnect user from Centrifugo.
func (n *Node) pubDisconnect(user string, reconnect bool) error {

	disconnect := &controlproto.Disconnect{
		User: user,
	}

	params, _ := n.ControlEncoder().EncodeDisconnect(disconnect)

	cmd := &controlproto.Command{
		UID:    n.uid,
		Method: controlproto.MethodTypeDisconnect,
		Params: params,
	}

	return <-n.publishControl(cmd)
}

// AddClient registers authenticated connection in clientConnectionHub
// this allows to make operations with user connection on demand.
func (n *Node) AddClient(c conns.Client) error {
	actionCount.WithLabelValues("add_client").Inc()
	return n.hub.Add(c)
}

// RemoveClient removes client connection from connection registry.
func (n *Node) RemoveClient(c conns.Client) error {
	actionCount.WithLabelValues("remove_client").Inc()
	return n.hub.Remove(c)
}

// AddSubscription registers subscription of connection on channel in both
// engine and clientSubscriptionHub.
func (n *Node) AddSubscription(ch string, c conns.Client) error {
	actionCount.WithLabelValues("add_subscription").Inc()
	first, err := n.hub.AddSub(ch, c)
	if err != nil {
		return err
	}
	if first {
		return n.engine.Subscribe(ch)
	}
	return nil
}

// RemoveSubscription removes subscription of connection on channel
// from both engine and clientSubscriptionHub.
func (n *Node) RemoveSubscription(ch string, c conns.Client) error {
	actionCount.WithLabelValues("remove_subscription").Inc()
	empty, err := n.hub.RemoveSub(ch, c)
	if err != nil {
		return err
	}
	if empty {
		return n.engine.Unsubscribe(ch)
	}
	return nil
}

// nodeCmd handles ping control command i.e. updates information about known nodes.
func (n *Node) nodeCmd(node *controlproto.Node) error {
	n.nodes.add(node)
	return nil
}

// Unsubscribe unsubscribes user from channel, if channel is equal to empty
// string then user will be unsubscribed from all channels.
func (n *Node) Unsubscribe(user string, ch string) error {

	if string(user) == "" {
		return proto.ErrBadRequest
	}

	if string(ch) != "" {
		_, ok := n.ChannelOpts(ch)
		if !ok {
			return proto.ErrNamespaceNotFound
		}
	}

	// First unsubscribe on this node.
	err := n.Hub().Unsubscribe(user, ch)
	if err != nil {
		return proto.ErrInternalServerError
	}
	// Second send unsubscribe control message to other nodes.
	err = n.pubUnsubscribe(user, ch)
	if err != nil {
		return proto.ErrInternalServerError
	}
	return nil
}

// Disconnect allows to close all user connections to Centrifugo.
func (n *Node) Disconnect(user string, reconnect bool) error {

	if string(user) == "" {
		return proto.ErrBadRequest
	}

	// first disconnect user from this node
	err := n.Hub().Disconnect(user, reconnect)
	if err != nil {
		return proto.ErrInternalServerError
	}
	// second send disconnect control message to other nodes
	err = n.pubDisconnect(user, reconnect)
	if err != nil {
		return proto.ErrInternalServerError
	}
	return nil
}

// namespaceName returns namespace name from channel if exists.
func (n *Node) namespaceName(ch string) string {
	cTrim := strings.TrimPrefix(ch, n.config.ChannelPrivatePrefix)
	if strings.Contains(cTrim, n.config.ChannelNamespaceBoundary) {
		parts := strings.SplitN(cTrim, n.config.ChannelNamespaceBoundary, 2)
		return parts[0]
	}
	return ""
}

// ChannelOpts returns channel options for channel using current channel config.
func (n *Node) ChannelOpts(ch string) (channel.Options, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.config.channelOpts(n.namespaceName(ch))
}

// AddPresence proxies presence adding to engine.
func (n *Node) AddPresence(ch string, uid string, info *proto.ClientInfo) error {
	n.mu.RLock()
	expire := int(n.config.ClientPresenceExpireInterval.Seconds())
	n.mu.RUnlock()
	actionCount.WithLabelValues("add_presence").Inc()
	return n.engine.AddPresence(ch, uid, info, expire)
}

// RemovePresence proxies presence removing to engine.
func (n *Node) RemovePresence(ch string, uid string) error {
	actionCount.WithLabelValues("remove_presence").Inc()
	return n.engine.RemovePresence(ch, uid)
}

// Presence returns a map with information about active clients in channel.
func (n *Node) Presence(ch string) (map[string]*proto.ClientInfo, error) {
	actionCount.WithLabelValues("presence").Inc()
	presence, err := n.engine.Presence(ch)
	if err != nil {
		return nil, err
	}
	return presence, nil
}

// History returns a slice of last messages published into project channel.
func (n *Node) History(ch string) ([]*proto.Publication, error) {
	actionCount.WithLabelValues("history").Inc()
	publications, err := n.engine.History(ch, engine.HistoryFilter{Limit: 0})
	if err != nil {
		return nil, err
	}
	return publications, nil
}

// RemoveHistory removes channel history.
func (n *Node) RemoveHistory(ch string) error {
	actionCount.WithLabelValues("remove_history").Inc()
	return n.engine.RemoveHistory(ch)
}

// LastMessageID return last message id for channel.
func (n *Node) LastMessageID(ch string) (string, error) {
	actionCount.WithLabelValues("last_message_id").Inc()
	publications, err := n.engine.History(ch, engine.HistoryFilter{Limit: 1})
	if err != nil {
		return "", err
	}
	if len(publications) == 0 {
		return "", nil
	}
	return publications[0].UID, nil
}

// PrivateChannel checks if channel private. In case of private channel
// subscription request must contain a proper signature.
func (n *Node) PrivateChannel(ch string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return strings.HasPrefix(string(ch), n.config.ChannelPrivatePrefix)
}

// UserAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it.
func (n *Node) UserAllowed(ch string, user string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if !strings.Contains(ch, n.config.ChannelUserBoundary) {
		return true
	}
	parts := strings.Split(ch, n.config.ChannelUserBoundary)
	allowedUsers := strings.Split(parts[len(parts)-1], n.config.ChannelUserSeparator)
	for _, allowedUser := range allowedUsers {
		if string(user) == allowedUser {
			return true
		}
	}
	return false
}

// ClientAllowed checks if client can subscribe on channel - as channel
// can contain special part in the end to indicate which client allowed
// to subscribe on it.
func (n *Node) ClientAllowed(ch string, client string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if !strings.Contains(ch, n.config.ChannelClientBoundary) {
		return true
	}
	parts := strings.Split(ch, n.config.ChannelClientBoundary)
	allowedClient := parts[len(parts)-1]
	if string(client) == allowedClient {
		return true
	}
	return false
}
