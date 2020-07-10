package centrifuge

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge/internal/controlpb"
	"github.com/centrifugal/centrifuge/internal/controlproto"
	"github.com/centrifugal/centrifuge/internal/dissolve"
	"github.com/centrifugal/centrifuge/internal/nowtime"

	"github.com/FZambia/eagle"
	"github.com/centrifugal/protocol"
	"github.com/google/uuid"
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
	// clientEvents to manage event handlers attached to node.
	clientEvents *clientEventHub
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

	subDissolver *dissolve.Dissolver

	// nowTimeGetter provides access to current time.
	nowTimeGetter nowtime.Getter
}

const (
	numSubLocks            = 16384
	numSubDissolverWorkers = 64
)

// New creates Node, the only required argument is config.
func New(c Config) (*Node, error) {
	uid := uuid.Must(uuid.NewRandom()).String()

	subLocks := make(map[int]*sync.Mutex, numSubLocks)
	for i := 0; i < numSubLocks; i++ {
		subLocks[i] = &sync.Mutex{}
	}

	if c.Name == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, err
		}
		c.Name = hostname
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
		clientEvents:   &clientEventHub{},
		subLocks:       subLocks,
		subDissolver:   dissolve.New(numSubDissolverWorkers),
		nowTimeGetter:  nowtime.Get,
	}

	if c.LogHandler != nil {
		n.logger = newLogger(c.LogLevel, c.LogHandler)
	}

	e, _ := NewMemoryEngine(n, MemoryEngineConfig{})
	n.SetEngine(e)
	return n, nil
}

var defaultChannelOptions = ChannelOptions{}

func (n *Node) channelOptions(ch string) (ChannelOptions, bool, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.config.ChannelOptionsFunc == nil {
		return defaultChannelOptions, true, nil
	}
	return n.config.ChannelOptionsFunc(ch)
}

// index chooses bucket number in range [0, numBuckets).
func index(s string, numBuckets int) int {
	if numBuckets == 1 {
		return 0
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(s))
	return int(hash.Sum64() % uint64(numBuckets))
}

func (n *Node) subLock(ch string) *sync.Mutex {
	return n.subLocks[index(ch, numSubLocks)]
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
	return n.subDissolver.Run()
}

// Log allows to log entry.
func (n *Node) Log(entry LogEntry) {
	n.logger.log(entry)
}

// LogEnabled allows to log entry.
func (n *Node) LogEnabled(level LogLevel) bool {
	return n.logger.enabled(level)
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
		defer func() { _ = closer.Close(ctx) }()
	}
	if n.historyManager != nil {
		if closer, ok := n.historyManager.(Closer); ok {
			defer func() { _ = closer.Close(ctx) }()
		}
	}
	if n.presenceManager != nil {
		if closer, ok := n.presenceManager.(Closer); ok {
			defer func() { _ = closer.Close(ctx) }()
		}
	}
	_ = n.subDissolver.Close()
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
	version := n.config.Version
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
	messagesReceivedCountControl.Inc()

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
	case controlpb.MethodTypeNode:
		cmd, err := n.controlDecoder.DecodeNode(params)
		if err != nil {
			n.logger.log(newLogEntry(LogLevelError, "error decoding node control params", map[string]interface{}{"error": err.Error()}))
			return err
		}
		return n.nodeCmd(cmd)
	case controlpb.MethodTypeUnsubscribe:
		cmd, err := n.controlDecoder.DecodeUnsubscribe(params)
		if err != nil {
			n.logger.log(newLogEntry(LogLevelError, "error decoding unsubscribe control params", map[string]interface{}{"error": err.Error()}))
			return err
		}
		return n.hub.unsubscribe(cmd.User, cmd.Channel)
	case controlpb.MethodTypeDisconnect:
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
func (n *Node) handlePublication(ch string, pub *protocol.Publication) error {
	messagesReceivedCountPublication.Inc()
	numSubscribers := n.hub.NumSubscribers(ch)
	hasCurrentSubscribers := numSubscribers > 0
	if !hasCurrentSubscribers {
		return nil
	}
	chOpts, found, err := n.channelOptions(ch)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	return n.hub.broadcastPublication(ch, pub, &chOpts)
}

// handleJoin handles join messages - i.e. broadcasts it to
// interested local clients subscribed to channel.
func (n *Node) handleJoin(ch string, join *protocol.Join) error {
	messagesReceivedCountJoin.Inc()
	hasCurrentSubscribers := n.hub.NumSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	return n.hub.broadcastJoin(ch, join)
}

// handleLeave handles leave messages - i.e. broadcasts it to
// interested local clients subscribed to channel.
func (n *Node) handleLeave(ch string, leave *protocol.Leave) error {
	messagesReceivedCountLeave.Inc()
	hasCurrentSubscribers := n.hub.NumSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	return n.hub.broadcastLeave(ch, leave)
}

func (n *Node) publish(ch string, data []byte, info *ClientInfo, opts ...PublishOption) (PublishResult, error) {
	chOpts, found, err := n.channelOptions(ch)
	if err != nil {
		return PublishResult{}, err
	}
	if !found {
		return PublishResult{}, ErrorUnknownChannel
	}

	publishOpts := &PublishOptions{}
	for _, opt := range opts {
		opt(publishOpts)
	}

	pub := &Publication{
		Data: data,
		Info: info,
	}

	messagesSentCountPublication.Inc()

	// If history enabled for channel we add Publication to history first and then
	// publish to Broker.
	if n.historyManager != nil && !publishOpts.SkipHistory && chOpts.HistorySize > 0 && chOpts.HistoryLifetime > 0 {
		streamPos, published, err := n.historyManager.AddHistory(ch, pub, &chOpts)
		if err != nil {
			return PublishResult{}, err
		}
		if !published {
			pub.Offset = streamPos.Offset
			// Publication added to history, no need to handle Publish error here.
			// In this case we rely on the fact that clients will automatically detect
			// missed publication and restore its state from history on reconnect.
			_ = n.broker.Publish(ch, pub, &chOpts)
		}
		return PublishResult{StreamPosition: streamPos}, nil
	}
	// If no history enabled - just publish to Broker. In this case we want to handle
	// error as message will be lost forever otherwise.
	err = n.broker.Publish(ch, pub, &chOpts)
	return PublishResult{}, err
}

// PublishResult ...
type PublishResult struct {
	StreamPosition
}

// Publish sends data to all clients subscribed on channel. All running nodes will
// receive it and send to all local channel subscribers.
//
// Data expected to be valid marshaled JSON or any binary payload.
// Connections that work over JSON protocol can not handle custom binary payloads.
// Connections that work over Protobuf protocol can work both with JSON and binary payloads.
//
// So the rule here: if you have channel subscribers that work using JSON
// protocol then you can not publish binary data to these channel.
//
// The returned PublishResult contains embedded StreamPosition that describes
// position inside stream Publication was added too. For channels without history
// enabled (i.e. when Publications only sent to PUB/SUB system) StreamPosition will
// be an empty struct (i.e. PublishResult.Offset will be zero).
func (n *Node) Publish(channel string, data []byte, opts ...PublishOption) (PublishResult, error) {
	return n.publish(channel, data, nil, opts...)
}

// publishJoin allows to publish join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (n *Node) publishJoin(ch string, info *ClientInfo, opts *ChannelOptions) error {
	if opts == nil {
		chOpts, found, err := n.channelOptions(ch)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		opts = &chOpts
	}
	messagesSentCountJoin.Inc()
	return n.broker.PublishJoin(ch, info, opts)
}

// publishLeave allows to publish join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (n *Node) publishLeave(ch string, info *ClientInfo, opts *ChannelOptions) error {
	if opts == nil {
		chOpts, found, err := n.channelOptions(ch)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		opts = &chOpts
	}
	messagesSentCountLeave.Inc()
	return n.broker.PublishLeave(ch, info, opts)
}

// publishControl publishes message into control channel so all running
// nodes will receive and handle it.
func (n *Node) publishControl(cmd *controlpb.Command) error {
	messagesSentCountControl.Inc()
	data, err := n.controlEncoder.EncodeCommand(cmd)
	if err != nil {
		return err
	}
	return n.broker.PublishControl(data)
}

func (n *Node) getMetrics(metrics eagle.Metrics) *controlpb.Metrics {
	return &controlpb.Metrics{
		Interval: n.config.NodeInfoMetricsAggregateInterval.Seconds(),
		Items:    metrics.Flatten("."),
	}
}

// pubNode sends control message to all nodes - this message
// contains information about current node.
func (n *Node) pubNode() error {
	n.mu.RLock()
	node := &controlpb.Node{
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

	cmd := &controlpb.Command{
		UID:    n.uid,
		Method: controlpb.MethodTypeNode,
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
	unsubscribe := &controlpb.Unsubscribe{
		User:    user,
		Channel: ch,
	}
	params, _ := n.controlEncoder.EncodeUnsubscribe(unsubscribe)
	cmd := &controlpb.Command{
		UID:    n.uid,
		Method: controlpb.MethodTypeUnsubscribe,
		Params: params,
	}
	return n.publishControl(cmd)
}

// pubDisconnect publishes disconnect control message to all nodes – so all
// nodes could disconnect user from server.
func (n *Node) pubDisconnect(user string, reconnect bool) error {
	// TODO: handle reconnect flag.
	disconnect := &controlpb.Disconnect{
		User: user,
	}
	params, _ := n.controlEncoder.EncodeDisconnect(disconnect)
	cmd := &controlpb.Command{
		UID:    n.uid,
		Method: controlpb.MethodTypeDisconnect,
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
// hub and engine.
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
			_, _ = n.hub.removeSub(ch, c)
			return err
		}
	}
	return nil
}

// removeSubscription removes subscription of connection on channel
// from hub and engine.
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
		submittedAt := time.Now()
		_ = n.subDissolver.Submit(func() error {
			timeSpent := time.Since(submittedAt)
			if timeSpent < time.Second {
				time.Sleep(time.Second - timeSpent)
			}
			mu := n.subLock(ch)
			mu.Lock()
			defer mu.Unlock()
			empty := n.hub.NumSubscribers(ch) == 0
			if empty {
				return n.broker.Unsubscribe(ch)
			}
			return nil
		})
	}
	return nil
}

// nodeCmd handles ping control command i.e. updates information about known nodes.
func (n *Node) nodeCmd(node *controlpb.Node) error {
	n.nodes.add(node)
	return nil
}

// Unsubscribe unsubscribes user from channel, if channel is equal to empty
// string then user will be unsubscribed from all channels.
func (n *Node) Unsubscribe(user string, ch string, opts ...UnsubscribeOption) error {
	unsubscribeOpts := &UnsubscribeOptions{}
	for _, opt := range opts {
		opt(unsubscribeOpts)
	}
	// First unsubscribe on this node.
	err := n.hub.unsubscribe(user, ch, opts...)
	if err != nil {
		return err
	}
	// Second send unsubscribe control message to other nodes.
	return n.pubUnsubscribe(user, ch)
}

// Disconnect allows to close all user connections through all nodes.
func (n *Node) Disconnect(user string, opts ...DisconnectOption) error {
	disconnectOpts := &DisconnectOptions{}
	for _, opt := range opts {
		opt(disconnectOpts)
	}
	// first disconnect user from this node
	err := n.hub.disconnect(user, disconnectOpts.Reconnect)
	if err != nil {
		return err
	}
	// second send disconnect control message to other nodes
	return n.pubDisconnect(user, disconnectOpts.Reconnect)
}

// addPresence proxies presence adding to engine.
func (n *Node) addPresence(ch string, uid string, info *ClientInfo) error {
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

// PresenceResult wraps presence.
type PresenceResult struct {
	Presence map[string]*ClientInfo
}

// Presence returns a map with information about active clients in channel.
func (n *Node) Presence(ch string) (PresenceResult, error) {
	if n.presenceManager == nil {
		return PresenceResult{}, ErrorNotAvailable
	}
	actionCount.WithLabelValues("presence").Inc()
	presence, err := n.presenceManager.Presence(ch)
	if err != nil {
		return PresenceResult{}, err
	}
	return PresenceResult{Presence: presence}, nil
}

func infoFromProto(v *protocol.ClientInfo) *ClientInfo {
	if v == nil {
		return nil
	}
	info := &ClientInfo{
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

func infoToProto(v *ClientInfo) *protocol.ClientInfo {
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

func pubToProto(pub *Publication) *protocol.Publication {
	if pub == nil {
		return nil
	}
	return &protocol.Publication{
		Offset: pub.Offset,
		Data:   pub.Data,
		Info:   infoToProto(pub.Info),
	}
}

func pubFromProto(pub *protocol.Publication) *Publication {
	if pub == nil {
		return nil
	}
	return &Publication{
		Offset: pub.GetOffset(),
		Data:   pub.Data,
		Info:   infoFromProto(pub.GetInfo()),
	}
}

// PresenceStatsResult wraps presence stats.
type PresenceStatsResult struct {
	PresenceStats
}

// PresenceStats returns presence stats from engine.
func (n *Node) PresenceStats(ch string) (PresenceStatsResult, error) {
	if n.presenceManager == nil {
		return PresenceStatsResult{}, nil
	}
	actionCount.WithLabelValues("presence_stats").Inc()
	presenceStats, err := n.presenceManager.PresenceStats(ch)
	if err != nil {
		return PresenceStatsResult{}, err
	}
	return PresenceStatsResult{PresenceStats: presenceStats}, nil
}

// HistoryResult contains Publications and current stream top StreamPosition.
type HistoryResult struct {
	// StreamPosition embedded here describes current stream top offset and epoch.
	StreamPosition
	// Publications extracted from history storage according to HistoryFilter.
	Publications []*Publication
}

// History allows to extract Publications in channel.
// The channel must belong to namespace where history is on.
func (n *Node) History(ch string, opts ...HistoryOption) (HistoryResult, error) {
	if n.historyManager == nil {
		return HistoryResult{}, ErrorNotAvailable
	}
	historyOpts := &HistoryOptions{}
	for _, opt := range opts {
		opt(historyOpts)
	}
	pubs, streamTop, err := n.historyManager.History(ch, HistoryFilter{
		Limit: historyOpts.Limit,
		Since: historyOpts.Since,
	})
	if err != nil {
		return HistoryResult{}, err
	}
	return HistoryResult{
		StreamPosition: streamTop,
		Publications:   pubs,
	}, nil
}

// fullHistory extracts full history in channel.
func (n *Node) fullHistory(ch string) (HistoryResult, error) {
	actionCount.WithLabelValues("history_full").Inc()
	return n.History(ch, WithNoLimit())
}

// recoverHistory recovers publications since last UID seen by client.
func (n *Node) recoverHistory(ch string, since StreamPosition) (HistoryResult, error) {
	actionCount.WithLabelValues("history_recover").Inc()
	return n.History(ch, WithNoLimit(), Since(since))
}

// streamTop returns current stream top position for channel.
func (n *Node) streamTop(ch string) (StreamPosition, error) {
	actionCount.WithLabelValues("history_stream_top").Inc()
	historyResult, err := n.History(ch)
	if err != nil {
		return StreamPosition{}, err
	}
	return historyResult.StreamPosition, nil
}

// RemoveHistory removes channel history.
func (n *Node) RemoveHistory(ch string) error {
	actionCount.WithLabelValues("history_remove").Inc()
	if n.historyManager == nil {
		return ErrorNotAvailable
	}
	return n.historyManager.RemoveHistory(ch)
}

type nodeRegistry struct {
	// mu allows to synchronize access to node registry.
	mu sync.RWMutex
	// currentUID keeps uid of current node
	currentUID string
	// nodes is a map with information about known nodes.
	nodes map[string]controlpb.Node
	// updates track time we last received ping from node. Used to clean up nodes map.
	updates map[string]int64
}

func newNodeRegistry(currentUID string) *nodeRegistry {
	return &nodeRegistry{
		currentUID: currentUID,
		nodes:      make(map[string]controlpb.Node),
		updates:    make(map[string]int64),
	}
}

func (r *nodeRegistry) list() []controlpb.Node {
	r.mu.RLock()
	nodes := make([]controlpb.Node, len(r.nodes))
	i := 0
	for _, info := range r.nodes {
		nodes[i] = info
		i++
	}
	r.mu.RUnlock()
	return nodes
}

func (r *nodeRegistry) get(uid string) controlpb.Node {
	r.mu.RLock()
	info := r.nodes[uid]
	r.mu.RUnlock()
	return info
}

func (r *nodeRegistry) add(info *controlpb.Node) {
	r.mu.Lock()
	if node, ok := r.nodes[info.UID]; ok {
		if info.Metrics != nil {
			r.nodes[info.UID] = *info
		} else {
			node.Version = info.Version
			node.NumChannels = info.NumChannels
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

// clientEventHub allows binding client event handlers.
// All clientEventHub methods are not goroutine-safe and supposed
// to be called once before Node Run called.
type clientEventHub struct {
	connectingHandler    ConnectingHandler
	connectHandler       ConnectHandler
	aliveHandler         AliveHandler
	disconnectHandler    DisconnectHandler
	subscribeHandler     SubscribeHandler
	unsubscribeHandler   UnsubscribeHandler
	publishHandler       PublishHandler
	refreshHandler       RefreshHandler
	subRefreshHandler    SubRefreshHandler
	rpcHandler           RPCHandler
	messageHandler       MessageHandler
	presenceHandler      PresenceHandler
	presenceStatsHandler PresenceStatsHandler
	historyHandler       HistoryHandler
}

// OnConnecting allows setting ConnectingHandler.
// ConnectingHandler will be called when client sends Connect command to server.
// In this handler server can reject connection or provide Credentials for it.
func (n *Node) OnConnecting(handler ConnectingHandler) {
	n.clientEvents.connectingHandler = handler
}

// OnConnect allows setting ConnectHandler.
// ConnectHandler called after client connection successfully established,
// authenticated and Connect Reply already sent to client. This is a place where
// application can start communicating with client.
func (n *Node) OnConnect(handler ConnectHandler) {
	n.clientEvents.connectHandler = handler
}

// OnAlive allows setting AliveHandler.
// AliveHandler called periodically for active client connection.
func (n *Node) OnAlive(h AliveHandler) {
	n.clientEvents.aliveHandler = h
}

// OnRefresh allows setting RefreshHandler.
// RefreshHandler called when it's time to refresh expiring client connection.
func (n *Node) OnRefresh(h RefreshHandler) {
	n.clientEvents.refreshHandler = h
}

// OnDisconnect allows setting DisconnectHandler.
// DisconnectHandler called when client disconnected from Node.
func (n *Node) OnDisconnect(h DisconnectHandler) {
	n.clientEvents.disconnectHandler = h
}

// OnMessage allows setting MessageHandler.
// MessageHandler called when client sent asynchronous message.
func (n *Node) OnMessage(h MessageHandler) {
	n.clientEvents.messageHandler = h
}

// OnRPC allows setting RPCHandler.
// RPCHandler will be executed on every incoming RPC call.
func (n *Node) OnRPC(h RPCHandler) {
	n.clientEvents.rpcHandler = h
}

// OnSubRefresh allows setting SubRefreshHandler.
// SubRefreshHandler called when it's time to refresh client subscription.
func (n *Node) OnSubRefresh(h SubRefreshHandler) {
	n.clientEvents.subRefreshHandler = h
}

// OnSubscribe allows setting SubscribeHandler.
// SubscribeHandler called when client subscribes on channel.
func (n *Node) OnSubscribe(h SubscribeHandler) {
	n.clientEvents.subscribeHandler = h
}

// OnUnsubscribe allows setting UnsubscribeHandler.
// UnsubscribeHandler called when client unsubscribes from channel.
func (n *Node) OnUnsubscribe(h UnsubscribeHandler) {
	n.clientEvents.unsubscribeHandler = h
}

// OnPublish allows setting PublishHandler.
// PublishHandler called when client publishes message into channel.
func (n *Node) OnPublish(h PublishHandler) {
	n.clientEvents.publishHandler = h
}

// OnPresence allows setting PresenceHandler.
// PresenceHandler called when Presence request from client received.
// At this moment you can only return a custom error or disconnect client.
func (n *Node) OnPresence(h PresenceHandler) {
	n.clientEvents.presenceHandler = h
}

// OnPresenceStats allows settings PresenceStatsHandler.
// PresenceStatsHandler called when PresenceStats request from client received.
// At this moment you can only return a custom error or disconnect client.
func (n *Node) OnPresenceStats(h PresenceStatsHandler) {
	n.clientEvents.presenceStatsHandler = h
}

// OnHistory allows settings HistoryHandler.
// HistoryHandler called when History request from client received.
// At this moment you can only return a custom error or disconnect client.
func (n *Node) OnHistory(h HistoryHandler) {
	n.clientEvents.historyHandler = h
}

type brokerEventHandler struct {
	node *Node
}

// HandlePublication coming from Engine.
func (h *brokerEventHandler) HandlePublication(ch string, pub *Publication) error {
	if pub == nil {
		panic("nil Publication received, this should never happen")
	}
	return h.node.handlePublication(ch, pubToProto(pub))
}

// HandleJoin coming from Engine.
func (h *brokerEventHandler) HandleJoin(ch string, info *ClientInfo) error {
	if info == nil {
		panic("nil join info received, this should never happen")
	}
	return h.node.handleJoin(ch, &protocol.Join{Info: *infoToProto(info)})
}

// HandleLeave coming from Engine.
func (h *brokerEventHandler) HandleLeave(ch string, info *ClientInfo) error {
	if info == nil {
		panic("nil leave info received, this should never happen")
	}
	return h.node.handleLeave(ch, &protocol.Leave{Info: *infoToProto(info)})
}

// HandleControl coming from Engine.
func (h *brokerEventHandler) HandleControl(data []byte) error {
	return h.node.handleControl(data)
}
