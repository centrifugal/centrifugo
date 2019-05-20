package centrifuge

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge/internal/proto"
	"github.com/centrifugal/centrifuge/internal/proto/controlproto"
	"github.com/centrifugal/centrifuge/internal/uuid"

	"github.com/FZambia/eagle"
	"github.com/prometheus/client_golang/prometheus"
)

// Node is a heart of centrifuge library – it internally keeps and manages
// client connections, maintains information about other centrifuge nodes,
// keeps useful references to things like engine, hub etc.
type Node struct {
	mu sync.RWMutex
	// unique id for this node.
	uid string
	// startedAt is unix time of node start.
	startedAt int64
	// config for node.
	config Config
	// hub to manage client connections.
	hub *Hub
	// broker is responsible for PUB/SUB mechanics.
	broker Broker
	// historyManager is responsible for managing channel Publication history.
	historyManager HistoryManager
	// presenceManager is responsible for presence information management.
	presenceManager PresenceManager
	// nodes contains registry of known nodes.
	nodes *nodeRegistry
	// shutdown is a flag which is only true when node is going to shut down.
	shutdown bool
	// shutdownCh is a channel which is closed when node shutdown initiated.
	shutdownCh chan struct{}
	// eventHub to manage event handlers binded to node.
	eventHub *nodeEventHub
	// logger allows to log throughout library code and proxy log entries to
	// configured log handler.
	logger *logger
	// cache control encoder in Node.
	controlEncoder controlproto.Encoder
	// cache control decoder in Node.
	controlDecoder controlproto.Decoder
	// subLocks synchronizes access to adding/removing subscriptions.
	subLocks map[int]*sync.Mutex

	metricsMu       sync.Mutex
	metricsExporter *eagle.Eagle
	metricsSnapshot *eagle.Metrics
}

const (
	numSubLocks = 16384
)

// New creates Node, the only required argument is config.
func New(c Config) (*Node, error) {
	uid := uuid.Must(uuid.NewV4()).String()

	subLocks := make(map[int]*sync.Mutex, numSubLocks)
	for i := 0; i < numSubLocks; i++ {
		subLocks[i] = &sync.Mutex{}
	}

	n := &Node{
		uid:            uid,
		nodes:          newNodeRegistry(uid),
		config:         c,
		hub:            newHub(),
		startedAt:      time.Now().Unix(),
		shutdownCh:     make(chan struct{}),
		logger:         nil,
		controlEncoder: controlproto.NewProtobufEncoder(),
		controlDecoder: controlproto.NewProtobufDecoder(),
		eventHub:       &nodeEventHub{},
		subLocks:       subLocks,
	}

	if c.LogHandler != nil {
		n.logger = newLogger(c.LogLevel, c.LogHandler)
	}

	e, _ := NewMemoryEngine(n, MemoryEngineConfig{})
	n.SetEngine(e)
	return n, nil
}

// index chooses bucket number in range [0, numBuckets).
func index(s string, numBuckets int) int {
	hash := fnv.New64a()
	hash.Write([]byte(s))
	return int(hash.Sum64() % uint64(numBuckets))
}

func (n *Node) subLock(ch string) *sync.Mutex {
	return n.subLocks[index(ch, numSubLocks)]
}

// Config returns a copy of node Config.
func (n *Node) Config() Config {
	n.mu.RLock()
	c := n.config
	n.mu.RUnlock()
	return c
}

// SetEngine binds Engine to node.
func (n *Node) SetEngine(e Engine) {
	n.broker = e.(Broker)
	n.historyManager = e.(HistoryManager)
	n.presenceManager = e.(PresenceManager)
}

// SetBroker allows to set Broker implementation to use.
func (n *Node) SetBroker(b Broker) {
	n.broker = b
}

// SetHistoryManager allows to set HistoryManager to use.
func (n *Node) SetHistoryManager(m HistoryManager) {
	n.historyManager = m
}

// SetPresenceManager allows to set PresenceManager to use.
func (n *Node) SetPresenceManager(m PresenceManager) {
	n.presenceManager = m
}

// Hub returns node's Hub.
func (n *Node) Hub() *Hub {
	return n.hub
}

// Reload node config.
func (n *Node) Reload(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.config = c
	return nil
}

// Run performs node startup actions. At moment must be called once on start
// after engine set to Node.
func (n *Node) Run() error {
	eventHandler := &brokerEventHandler{n}
	if err := n.broker.Run(eventHandler); err != nil {
		return err
	}
	err := n.initMetrics()
	if err != nil {
		n.logger.log(newLogEntry(LogLevelError, "error on init metrics", map[string]interface{}{"error": err.Error()}))
		return err
	}
	err = n.pubNode()
	if err != nil {
		n.logger.log(newLogEntry(LogLevelError, "error publishing node control command", map[string]interface{}{"error": err.Error()}))
		return err
	}
	go n.sendNodePing()
	go n.cleanNodeInfo()
	go n.updateMetrics()
	return nil
}

// Log allows to log entry.
func (n *Node) Log(entry LogEntry) {
	n.logger.log(entry)
}

// LogEnabled allows to log entry.
func (n *Node) LogEnabled(level LogLevel) bool {
	return n.logger.enabled(level)
}

// On allows access to NodeEventHub.
func (n *Node) On() NodeEventHub {
	return n.eventHub
}

// Shutdown sets shutdown flag to Node so handlers could stop accepting
// new requests and disconnects clients with shutdown reason.
func (n *Node) Shutdown(ctx context.Context) error {
	n.mu.Lock()
	if n.shutdown {
		n.mu.Unlock()
		return nil
	}
	n.shutdown = true
	close(n.shutdownCh)
	n.mu.Unlock()
	if closer, ok := n.broker.(Closer); ok {
		defer closer.Close(ctx)
	}
	if closer, ok := n.historyManager.(Closer); ok {
		defer closer.Close(ctx)
	}
	if closer, ok := n.presenceManager.(Closer); ok {
		defer closer.Close(ctx)
	}
	return n.hub.shutdown(ctx)
}

// NotifyShutdown returns a channel which will be closed on node shutdown.
func (n *Node) NotifyShutdown() chan struct{} {
	return n.shutdownCh
}

func (n *Node) updateGauges() {
	numClientsGauge.Set(float64(n.hub.NumClients()))
	numUsersGauge.Set(float64(n.hub.NumUsers()))
	numChannelsGauge.Set(float64(n.hub.NumChannels()))
	version := n.Config().Version
	if version == "" {
		version = "_"
	}
	buildInfoGauge.WithLabelValues(version).Set(1)
}

func (n *Node) updateMetrics() {
	n.updateGauges()
	for {
		select {
		case <-n.shutdownCh:
			return
		case <-time.After(10 * time.Second):
			n.updateGauges()
		}
	}
}

// Centrifuge library uses Prometheus metrics for instrumentation. But we also try to
// aggregate Prometheus metrics periodically and share this information between nodes.
// At moment this allows to show metrics in Centrifugo admin interface.
func (n *Node) initMetrics() error {
	if n.config.NodeInfoMetricsAggregateInterval == 0 {
		return nil
	}
	metricsSink := make(chan eagle.Metrics)
	n.metricsExporter = eagle.New(eagle.Config{
		Gatherer: prometheus.DefaultGatherer,
		Interval: n.config.NodeInfoMetricsAggregateInterval,
		Sink:     metricsSink,
	})
	metrics, err := n.metricsExporter.Export()
	if err != nil {
		return err
	}
	n.metricsMu.Lock()
	n.metricsSnapshot = &metrics
	n.metricsMu.Unlock()
	go func() {
		for {
			select {
			case <-n.NotifyShutdown():
				return
			case metrics := <-metricsSink:
				n.metricsMu.Lock()
				n.metricsSnapshot = &metrics
				n.metricsMu.Unlock()
			}
		}
	}()
	return nil
}

func (n *Node) sendNodePing() {
	for {
		select {
		case <-n.shutdownCh:
			return
		case <-time.After(nodeInfoPublishInterval):
			err := n.pubNode()
			if err != nil {
				n.logger.log(newLogEntry(LogLevelError, "error publishing node control command", map[string]interface{}{"error": err.Error()}))
			}
		}
	}
}

func (n *Node) cleanNodeInfo() {
	for {
		select {
		case <-n.shutdownCh:
			return
		case <-time.After(nodeInfoCleanInterval):
			n.mu.RLock()
			delay := nodeInfoMaxDelay
			n.mu.RUnlock()
			n.nodes.clean(delay)
		}
	}
}

// Channels returns list of all channels currently active across on all nodes.
// This is a snapshot of state mostly useful for understanding what's going on
// with system.
func (n *Node) Channels() ([]string, error) {
	return n.broker.Channels()
}

// Info contains information about all known server nodes.
type Info struct {
	Nodes []NodeInfo
}

// Metrics aggregation over time interval for node.
type Metrics struct {
	Interval float64
	Items    map[string]float64
}

// NodeInfo contains information about node.
type NodeInfo struct {
	UID         string
	Name        string
	Version     string
	NumClients  uint32
	NumUsers    uint32
	NumChannels uint32
	Uptime      uint32
	Metrics     *Metrics
}

// Info returns aggregated stats from all nodes.
func (n *Node) Info() (Info, error) {
	nodes := n.nodes.list()
	nodeResults := make([]NodeInfo, len(nodes))
	for i, nd := range nodes {
		info := NodeInfo{
			UID:         nd.UID,
			Name:        nd.Name,
			Version:     nd.Version,
			NumClients:  nd.NumClients,
			NumUsers:    nd.NumUsers,
			NumChannels: nd.NumChannels,
			Uptime:      nd.Uptime,
		}
		if nd.Metrics != nil {
			info.Metrics = &Metrics{
				Interval: nd.Metrics.Interval,
				Items:    nd.Metrics.Items,
			}
		}
		nodeResults[i] = info
	}

	return Info{
		Nodes: nodeResults,
	}, nil
}

// handleControl handles messages from control channel - control messages used for internal
// communication between nodes to share state or proto.
func (n *Node) handleControl(data []byte) error {
	messagesReceivedCount.WithLabelValues("control").Inc()

	cmd, err := n.controlDecoder.DecodeCommand(data)
	if err != nil {
		n.logger.log(newLogEntry(LogLevelError, "error decoding control command", map[string]interface{}{"error": err.Error()}))
		return err
	}

	if cmd.UID == n.uid {
		// Sent by this node.
		return nil
	}

	method := cmd.Method
	params := cmd.Params

	switch method {
	case controlproto.MethodTypeNode:
		cmd, err := n.controlDecoder.DecodeNode(params)
		if err != nil {
			n.logger.log(newLogEntry(LogLevelError, "error decoding node control params", map[string]interface{}{"error": err.Error()}))
			return err
		}
		return n.nodeCmd(cmd)
	case controlproto.MethodTypeUnsubscribe:
		cmd, err := n.controlDecoder.DecodeUnsubscribe(params)
		if err != nil {
			n.logger.log(newLogEntry(LogLevelError, "error decoding unsubscribe control params", map[string]interface{}{"error": err.Error()}))
			return err
		}
		return n.hub.unsubscribe(cmd.User, cmd.Channel)
	case controlproto.MethodTypeDisconnect:
		cmd, err := n.controlDecoder.DecodeDisconnect(params)
		if err != nil {
			n.logger.log(newLogEntry(LogLevelError, "error decoding disconnect control params", map[string]interface{}{"error": err.Error()}))
			return err
		}
		return n.hub.disconnect(cmd.User, false)
	default:
		n.logger.log(newLogEntry(LogLevelError, "unknown control message method", map[string]interface{}{"method": method}))
		return fmt.Errorf("control method not found: %d", method)
	}
}

// handlePublication handles messages published into channel and
// coming from engine. The goal of method is to deliver this message
// to all clients on this node currently subscribed to channel.
func (n *Node) handlePublication(ch string, pub *Publication) error {
	messagesReceivedCount.WithLabelValues("publication").Inc()
	numSubscribers := n.hub.NumSubscribers(ch)
	hasCurrentSubscribers := numSubscribers > 0
	if !hasCurrentSubscribers {
		return nil
	}
	chOpts, ok := n.ChannelOpts(ch)
	if !ok {
		return ErrNoChannelOptions
	}
	return n.hub.broadcastPublication(ch, pub, &chOpts)
}

// handleJoin handles join messages - i.e. broadcasts it to
// interested local clients subscribed to channel.
func (n *Node) handleJoin(ch string, join *proto.Join) error {
	messagesReceivedCount.WithLabelValues("join").Inc()
	hasCurrentSubscribers := n.hub.NumSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	return n.hub.broadcastJoin(ch, join)
}

// handleLeave handles leave messages - i.e. broadcasts it to
// interested local clients subscribed to channel.
func (n *Node) handleLeave(ch string, leave *proto.Leave) error {
	messagesReceivedCount.WithLabelValues("leave").Inc()
	hasCurrentSubscribers := n.hub.NumSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	return n.hub.broadcastLeave(ch, leave)
}

func (n *Node) publish(ch string, data []byte, info *ClientInfo, opts ...PublishOption) error {
	chOpts, ok := n.ChannelOpts(ch)
	if !ok {
		return ErrNoChannelOptions
	}

	publishOpts := &PublishOptions{}
	for _, opt := range opts {
		opt(publishOpts)
	}

	pub := &Publication{
		Data: data,
		Info: info,
	}

	messagesSentCount.WithLabelValues("publication").Inc()

	// If history enabled for channel we add Publication to history first and then
	// publish to Broker.
	if n.historyManager != nil && !publishOpts.SkipHistory && chOpts.HistorySize > 0 && chOpts.HistoryLifetime > 0 {
		pub, err := n.historyManager.AddHistory(ch, pub, &chOpts)
		if err != nil {
			return err
		}
		if pub != nil {
			// Publication added to history, no need to handle Publish error here.
			// In this case we rely on the fact that clients will eventually restore
			// Publication from history.
			n.broker.Publish(ch, pub, &chOpts)
		}
		return nil
	}
	// If no history enabled - just publish to Broker. In this case we want to handle
	// error as message will be lost forever otherwise.
	return n.broker.Publish(ch, pub, &chOpts)
}

// Publish sends data to all clients subscribed on channel. All running nodes
// will receive it and will send it to all clients on node subscribed on channel.
func (n *Node) Publish(ch string, data []byte, opts ...PublishOption) error {
	return n.publish(ch, data, nil, opts...)
}

var (
	// ErrNoChannelOptions returned when operation can't be performed because no
	// appropriate channel options were found for channel.
	ErrNoChannelOptions = errors.New("no channel options found")
)

// publishJoin allows to publish join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (n *Node) publishJoin(ch string, join *proto.Join, opts *ChannelOptions) error {
	if opts == nil {
		chOpts, ok := n.ChannelOpts(ch)
		if !ok {
			return ErrorNamespaceNotFound
		}
		opts = &chOpts
	}
	messagesSentCount.WithLabelValues("join").Inc()
	return n.broker.PublishJoin(ch, join, opts)
}

// publishLeave allows to publish join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (n *Node) publishLeave(ch string, leave *proto.Leave, opts *ChannelOptions) error {
	if opts == nil {
		chOpts, ok := n.ChannelOpts(ch)
		if !ok {
			return ErrorNamespaceNotFound
		}
		opts = &chOpts
	}
	messagesSentCount.WithLabelValues("leave").Inc()
	return n.broker.PublishLeave(ch, leave, opts)
}

// publishControl publishes message into control channel so all running
// nodes will receive and handle it.
func (n *Node) publishControl(cmd *controlproto.Command) error {
	messagesSentCount.WithLabelValues("control").Inc()
	data, err := n.controlEncoder.EncodeCommand(cmd)
	if err != nil {
		return err
	}
	return n.broker.PublishControl(data)
}

func (n *Node) getMetrics(metrics eagle.Metrics) *controlproto.Metrics {
	return &controlproto.Metrics{
		Interval: n.config.NodeInfoMetricsAggregateInterval.Seconds(),
		Items:    metrics.Flatten("."),
	}
}

// pubNode sends control message to all nodes - this message
// contains information about current node.
func (n *Node) pubNode() error {
	n.mu.RLock()
	node := &controlproto.Node{
		UID:         n.uid,
		Name:        n.config.Name,
		Version:     n.config.Version,
		NumClients:  uint32(n.hub.NumClients()),
		NumUsers:    uint32(n.hub.NumUsers()),
		NumChannels: uint32(n.hub.NumChannels()),
		Uptime:      uint32(time.Now().Unix() - n.startedAt),
	}

	n.metricsMu.Lock()
	if n.metricsSnapshot != nil {
		node.Metrics = n.getMetrics(*n.metricsSnapshot)
	}
	// We only send metrics once when updated.
	n.metricsSnapshot = nil
	n.metricsMu.Unlock()

	n.mu.RUnlock()

	params, _ := n.controlEncoder.EncodeNode(node)

	cmd := &controlproto.Command{
		UID:    n.uid,
		Method: controlproto.MethodTypeNode,
		Params: params,
	}

	err := n.nodeCmd(node)
	if err != nil {
		n.logger.log(newLogEntry(LogLevelError, "error handling node command", map[string]interface{}{"error": err.Error()}))
	}

	return n.publishControl(cmd)
}

// pubUnsubscribe publishes unsubscribe control message to all nodes – so all
// nodes could unsubscribe user from channel.
func (n *Node) pubUnsubscribe(user string, ch string) error {
	unsubscribe := &controlproto.Unsubscribe{
		User:    user,
		Channel: ch,
	}
	params, _ := n.controlEncoder.EncodeUnsubscribe(unsubscribe)
	cmd := &controlproto.Command{
		UID:    n.uid,
		Method: controlproto.MethodTypeUnsubscribe,
		Params: params,
	}
	return n.publishControl(cmd)
}

// pubDisconnect publishes disconnect control message to all nodes – so all
// nodes could disconnect user from Centrifugo.
func (n *Node) pubDisconnect(user string, reconnect bool) error {
	disconnect := &controlproto.Disconnect{
		User: user,
	}
	params, _ := n.controlEncoder.EncodeDisconnect(disconnect)
	cmd := &controlproto.Command{
		UID:    n.uid,
		Method: controlproto.MethodTypeDisconnect,
		Params: params,
	}
	return n.publishControl(cmd)
}

// addClient registers authenticated connection in clientConnectionHub
// this allows to make operations with user connection on demand.
func (n *Node) addClient(c *Client) error {
	actionCount.WithLabelValues("add_client").Inc()
	return n.hub.add(c)
}

// removeClient removes client connection from connection registry.
func (n *Node) removeClient(c *Client) error {
	actionCount.WithLabelValues("remove_client").Inc()
	return n.hub.remove(c)
}

// addSubscription registers subscription of connection on channel in both
// engine and clientSubscriptionHub.
func (n *Node) addSubscription(ch string, c *Client) error {
	actionCount.WithLabelValues("add_subscription").Inc()
	mu := n.subLock(ch)
	mu.Lock()
	defer mu.Unlock()
	first, err := n.hub.addSub(ch, c)
	if err != nil {
		return err
	}
	if first {
		err := n.broker.Subscribe(ch)
		if err != nil {
			n.hub.removeSub(ch, c)
			return err
		}
	}
	return nil
}

// removeSubscription removes subscription of connection on channel
// from both engine and clientSubscriptionHub.
func (n *Node) removeSubscription(ch string, c *Client) error {
	actionCount.WithLabelValues("remove_subscription").Inc()
	mu := n.subLock(ch)
	mu.Lock()
	defer mu.Unlock()
	empty, err := n.hub.removeSub(ch, c)
	if err != nil {
		return err
	}
	if empty {
		return n.broker.Unsubscribe(ch)
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
	// First unsubscribe on this node.
	err := n.hub.unsubscribe(user, ch)
	if err != nil {
		return err
	}
	// Second send unsubscribe control message to other nodes.
	return n.pubUnsubscribe(user, ch)
}

// Disconnect allows to close all user connections to Centrifugo.
func (n *Node) Disconnect(user string, reconnect bool) error {
	// first disconnect user from this node
	err := n.hub.disconnect(user, reconnect)
	if err != nil {
		return err
	}
	// second send disconnect control message to other nodes
	return n.pubDisconnect(user, reconnect)
}

// namespaceName returns namespace name from channel if exists.
func (n *Node) namespaceName(ch string) string {
	cTrim := strings.TrimPrefix(ch, n.config.ChannelPrivatePrefix)
	if n.config.ChannelNamespaceBoundary != "" && strings.Contains(cTrim, n.config.ChannelNamespaceBoundary) {
		parts := strings.SplitN(cTrim, n.config.ChannelNamespaceBoundary, 2)
		return parts[0]
	}
	return ""
}

// ChannelOpts returns channel options for channel using current channel config.
func (n *Node) ChannelOpts(ch string) (ChannelOptions, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.config.channelOpts(n.namespaceName(ch))
}

// addPresence proxies presence adding to engine.
func (n *Node) addPresence(ch string, uid string, info *proto.ClientInfo) error {
	if n.presenceManager == nil {
		return nil
	}
	n.mu.RLock()
	expire := n.config.ClientPresenceExpireInterval
	n.mu.RUnlock()
	actionCount.WithLabelValues("add_presence").Inc()
	return n.presenceManager.AddPresence(ch, uid, info, expire)
}

// removePresence proxies presence removing to engine.
func (n *Node) removePresence(ch string, uid string) error {
	if n.presenceManager == nil {
		return nil
	}
	actionCount.WithLabelValues("remove_presence").Inc()
	return n.presenceManager.RemovePresence(ch, uid)
}

// Presence returns a map with information about active clients in channel.
func (n *Node) Presence(ch string) (map[string]*ClientInfo, error) {
	if n.presenceManager == nil {
		return nil, nil
	}
	actionCount.WithLabelValues("presence").Inc()
	presence, err := n.presenceManager.Presence(ch)
	if err != nil {
		return nil, err
	}
	return presence, nil
}

// PresenceStats returns presence stats from engine.
func (n *Node) PresenceStats(ch string) (PresenceStats, error) {
	if n.presenceManager == nil {
		return PresenceStats{}, nil
	}
	actionCount.WithLabelValues("presence_stats").Inc()
	return n.presenceManager.PresenceStats(ch)
}

// History returns a slice of last messages published into project channel.
func (n *Node) History(ch string) ([]*Publication, error) {
	actionCount.WithLabelValues("history").Inc()
	pubs, _, err := n.historyManager.History(ch, HistoryFilter{
		Limit: -1,
		Since: nil,
	})
	return pubs, err
}

// recoverHistory recovers publications since last UID seen by client.
func (n *Node) recoverHistory(ch string, since RecoveryPosition) ([]*Publication, RecoveryPosition, error) {
	actionCount.WithLabelValues("recover_history").Inc()
	return n.historyManager.History(ch, HistoryFilter{
		Limit: -1,
		Since: &since,
	})
}

// RemoveHistory removes channel history.
func (n *Node) RemoveHistory(ch string) error {
	actionCount.WithLabelValues("remove_history").Inc()
	return n.historyManager.RemoveHistory(ch)
}

// currentRecoveryState returns current recovery state for channel.
func (n *Node) currentRecoveryState(ch string) (RecoveryPosition, error) {
	actionCount.WithLabelValues("history_recovery_state").Inc()
	_, recoveryPosition, err := n.historyManager.History(ch, HistoryFilter{
		Limit: 0,
		Since: nil,
	})
	return recoveryPosition, err
}

// privateChannel checks if channel private. In case of private channel
// subscription request must contain a proper signature.
func (n *Node) privateChannel(ch string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.config.ChannelPrivatePrefix == "" {
		return false
	}
	return strings.HasPrefix(ch, n.config.ChannelPrivatePrefix)
}

// userAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it.
func (n *Node) userAllowed(ch string, user string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	userBoundary := n.config.ChannelUserBoundary
	userSeparator := n.config.ChannelUserSeparator
	if userBoundary == "" {
		return true
	}
	if !strings.Contains(ch, userBoundary) {
		return true
	}
	parts := strings.Split(ch, userBoundary)
	if userSeparator == "" {
		return parts[len(parts)-1] == user
	}
	allowedUsers := strings.Split(parts[len(parts)-1], userSeparator)
	for _, allowedUser := range allowedUsers {
		if user == allowedUser {
			return true
		}
	}
	return false
}

type nodeRegistry struct {
	// mu allows to synchronize access to node registry.
	mu sync.RWMutex
	// currentUID keeps uid of current node
	currentUID string
	// nodes is a map with information about known nodes.
	nodes map[string]controlproto.Node
	// updates track time we last received ping from node. Used to clean up nodes map.
	updates map[string]int64
}

func newNodeRegistry(currentUID string) *nodeRegistry {
	return &nodeRegistry{
		currentUID: currentUID,
		nodes:      make(map[string]controlproto.Node),
		updates:    make(map[string]int64),
	}
}

func (r *nodeRegistry) list() []controlproto.Node {
	r.mu.RLock()
	nodes := make([]controlproto.Node, len(r.nodes))
	i := 0
	for _, info := range r.nodes {
		nodes[i] = info
		i++
	}
	r.mu.RUnlock()
	return nodes
}

func (r *nodeRegistry) get(uid string) controlproto.Node {
	r.mu.RLock()
	info := r.nodes[uid]
	r.mu.RUnlock()
	return info
}

func (r *nodeRegistry) add(info *controlproto.Node) {
	r.mu.Lock()
	if node, ok := r.nodes[info.UID]; ok {
		if info.Metrics != nil {
			r.nodes[info.UID] = *info
		} else {
			node.Version = info.Version
			node.NumClients = info.NumClients
			node.NumUsers = info.NumUsers
			node.Uptime = info.Uptime
			r.nodes[info.UID] = node
		}
	} else {
		r.nodes[info.UID] = *info
	}
	r.updates[info.UID] = time.Now().Unix()
	r.mu.Unlock()
}

func (r *nodeRegistry) clean(delay time.Duration) {
	r.mu.Lock()
	for uid := range r.nodes {
		if uid == r.currentUID {
			// No need to clean info for current node.
			continue
		}
		updated, ok := r.updates[uid]
		if !ok {
			// As we do all operations with nodes under lock this should never happen.
			delete(r.nodes, uid)
			continue
		}
		if time.Now().Unix()-updated > int64(delay.Seconds()) {
			// Too many seconds since this node have been last seen - remove it from map.
			delete(r.nodes, uid)
			delete(r.updates, uid)
		}
	}
	r.mu.Unlock()
}

// NodeEventHub can deal with events binded to Node.
// All its methods are not goroutine-safe as handlers must be
// registered once before Node Run method called.
type NodeEventHub interface {
	// Auth happens when client sends Connect command to server. In this handler client
	// can reject connection or provide Credentials for it.
	ClientConnecting(handler ConnectingHandler)
	// Connect called after client connection has been successfully established,
	// authenticated and connect reply already sent to client. This is a place
	// where application should set all required connection event callbacks and
	// can start communicating with client.
	ClientConnected(handler ConnectedHandler)
	// ClientRefresh called when it's time to refresh expiring client connection.
	ClientRefresh(handler RefreshHandler)
}

// nodeEventHub can deal with events binded to Node.
// All its methods are not goroutine-safe.
type nodeEventHub struct {
	connectingHandler ConnectingHandler
	connectedHandler  ConnectedHandler
	refreshHandler    RefreshHandler
}

// ClientConnecting ...
func (h *nodeEventHub) ClientConnecting(handler ConnectingHandler) {
	h.connectingHandler = handler
}

// ClientConnected allows to set ConnectedHandler.
func (h *nodeEventHub) ClientConnected(handler ConnectedHandler) {
	h.connectedHandler = handler
}

// ClientRefresh allows to set RefreshHandler.
func (h *nodeEventHub) ClientRefresh(handler RefreshHandler) {
	h.refreshHandler = handler
}

type brokerEventHandler struct {
	node *Node
}

// HandlePublication ...
func (h *brokerEventHandler) HandlePublication(ch string, pub *Publication) error {
	return h.node.handlePublication(ch, pub)
}

// HandleJoin ...
func (h *brokerEventHandler) HandleJoin(ch string, join *Join) error {
	return h.node.handleJoin(ch, join)
}

// HandleLeave ...
func (h *brokerEventHandler) HandleLeave(ch string, leave *Leave) error {
	return h.node.handleLeave(ch, leave)
}

// HandleControl ...
func (h *brokerEventHandler) HandleControl(data []byte) error {
	return h.node.handleControl(data)
}
