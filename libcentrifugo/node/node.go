// Package node is a real-time core for Centrifugo server.
package node

import (
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/metrics"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/centrifugal/centrifugo/libcentrifugo/server"
	"github.com/satori/go.uuid"
)

type RunOptions struct {
	Engine   engine.Engine
	Servers  map[string]server.Server
	Mediator Mediator
}

// Node is a heart of Centrifugo – it internally manages client and admin hubs,
// maintains information about other Centrifugo nodes, keeps references to
// config, engine, metrics etc.
type Node struct {
	// TODO: make private.
	sync.RWMutex

	// version
	version string

	// unique id for this node.
	uid string

	// started is unix time of node start.
	started int64

	// nodes is a map with information about nodes known.
	nodes map[string]proto.NodeInfo

	// nodesMu allows to synchronize access to nodes.
	nodesMu sync.Mutex

	// hub to manage client connections.
	clients conns.ClientHub

	// hub to manage admin connections.
	admins conns.AdminHub

	// config for application.
	config *Config

	// engine to use - in memory or redis.
	engine engine.Engine

	// servers contains list of servers connected to this node.
	servers map[string]server.Server

	// mediator allows integrate libcentrifugo Node with external go code.
	mediator Mediator

	// shutdown is a flag which is only true when application is going to shut down.
	shutdown bool

	// shutdownCh is a channel which is closed when shutdown happens.
	shutdownCh chan struct{}

	// save metrics snapshot until next metrics interval.
	metricsSnapshot map[string]int64

	// protect access to metrics snapshot.
	metricsMu sync.RWMutex
}

// global metrics registry pointing to the same Registry plugin package uses.
var metricsRegistry *metrics.Registry

func init() {
	metricsRegistry = metrics.DefaultRegistry

	metricsRegistry.RegisterCounter("num_msg_published", metrics.NewCounter())
	metricsRegistry.RegisterCounter("num_msg_queued", metrics.NewCounter())
	metricsRegistry.RegisterCounter("num_msg_sent", metrics.NewCounter())
	metricsRegistry.RegisterCounter("num_api_requests", metrics.NewCounter())
	metricsRegistry.RegisterCounter("num_client_requests", metrics.NewCounter())
	metricsRegistry.RegisterCounter("bytes_client_in", metrics.NewCounter())
	metricsRegistry.RegisterCounter("bytes_client_out", metrics.NewCounter())
	metricsRegistry.RegisterCounter("num_msg_published", metrics.NewCounter())

	metricsRegistry.RegisterGauge("memory_sys", metrics.NewGauge())
	metricsRegistry.RegisterGauge("cpu_usage", metrics.NewGauge())
	metricsRegistry.RegisterGauge("num_goroutine", metrics.NewGauge())
	metricsRegistry.RegisterGauge("num_clients", metrics.NewGauge())
	metricsRegistry.RegisterGauge("num_unique_clients", metrics.NewGauge())
	metricsRegistry.RegisterGauge("num_channels", metrics.NewGauge())
	metricsRegistry.RegisterGauge("gomaxprocs", metrics.NewGauge())
	metricsRegistry.RegisterGauge("num_cpu", metrics.NewGauge())

	quantiles := []float64{50, 90, 99, 99.99}
	var minValue int64 = 1        // record latencies in microseconds, min resolution 1mks.
	var maxValue int64 = 60000000 // record latencies in microseconds, max resolution 60s.
	numBuckets := 15              // histograms will be rotated every time we updating snapshot.
	sigfigs := 3
	metricsRegistry.RegisterHDRHistogram("client_api", metrics.NewHDRHistogram(numBuckets, minValue, maxValue, sigfigs, quantiles, "microseconds"))
}

// New creates Node, the only required argument is config.
func New(version string, c *Config) *Node {
	n := &Node{
		version:         version,
		uid:             uuid.NewV4().String(),
		config:          c,
		clients:         conns.NewClientHub(),
		admins:          conns.NewAdminHub(),
		nodes:           make(map[string]proto.NodeInfo),
		started:         time.Now().Unix(),
		metricsSnapshot: make(map[string]int64),
		shutdownCh:      make(chan struct{}),
	}

	// Create initial snapshot with empty values.
	n.metricsMu.Lock()
	n.metricsSnapshot = n.getSnapshotMetrics()
	n.metricsMu.Unlock()

	return n
}

// Config returns a copy of node Config.
func (n *Node) Config() Config {
	n.RLock()
	c := *n.config
	n.RUnlock()
	return c
}

// SetConfig binds config to application.
func (n *Node) SetConfig(c *Config) {
	n.Lock()
	defer n.Unlock()
	n.config = c
}

func (n *Node) Version() string {
	return n.version
}

// Reload node.
func (n *Node) Reload(getter config.Getter) error {
	if validator, ok := n.engine.(config.Validator); ok {
		err := validator.Validate(getter)
		if err != nil {
			return err
		}
	}
	for _, server := range n.servers {
		if validator, ok := server.(config.Validator); ok {
			err := validator.Validate(getter)
			if err != nil {
				return err
			}
		}
	}

	c := NewConfig(getter)
	if err := c.Validate(); err != nil {
		return err
	}
	n.SetConfig(c)

	if reloader, ok := n.engine.(config.Reloader); ok {
		err := reloader.Reload(getter)
		if err != nil {
			logger.ERROR.Printf("Error reloading engine: %v", err)
		}
	}

	for srvName, server := range n.servers {
		if reloader, ok := server.(config.Reloader); ok {
			err := reloader.Reload(getter)
			if err != nil {
				logger.ERROR.Printf("Error reloading server %s: %v", srvName, err)
			}
		}
	}

	return nil
}

// Config returns a copy of node Config.
func (n *Node) Engine() engine.Engine {
	return n.engine
}

// Config returns a copy of node Config.
func (n *Node) Mediator() Mediator {
	return n.mediator
}

// ClientHub.
func (n *Node) ClientHub() conns.ClientHub {
	return n.clients
}

// AdminHub.
func (n *Node) AdminHub() conns.AdminHub {
	return n.admins
}

// Notify shutdown returns a channel which will be closed on node shutdown.
func (n *Node) NotifyShutdown() chan struct{} {
	return n.shutdownCh
}

// Run performs all startup actions. At moment must be called once on start
// after engine and structure set.
func (n *Node) Run(opts *RunOptions) error {
	n.Lock()
	n.engine = opts.Engine
	n.servers = opts.Servers
	n.mediator = opts.Mediator
	n.Unlock()

	if err := n.engine.Run(); err != nil {
		return err
	}

	err := n.pubPing()
	if err != nil {
		logger.CRITICAL.Println(err)
	}
	go n.sendNodePingMsg()

	go n.cleanNodeInfo()

	go n.updateMetrics()

	config := n.Config()
	if config.Insecure {
		logger.WARN.Println("Running in INSECURE client mode")
	}
	if config.InsecureAPI {
		logger.WARN.Println("Running in INSECURE API mode")
	}
	if config.InsecureAdmin {
		logger.WARN.Println("Running in INSECURE admin mode")
	}
	if config.Debug {
		logger.WARN.Println("Running in DEBUG mode")
	}

	for srvName, srv := range n.servers {
		logger.INFO.Printf("Starting %s server", srvName)
		go srv.Run()
	}

	return nil
}

// Shutdown sets shutdown flag and does various clean ups.
func (n *Node) Shutdown() error {
	n.Lock()
	if n.shutdown {
		n.Unlock()
		return nil
	}
	n.shutdown = true
	close(n.shutdownCh)
	n.Unlock()
	for srvName, srv := range n.servers {
		logger.INFO.Printf("Shutting down %s server", srvName)
		if err := srv.Shutdown(); err != nil {
			logger.ERROR.Printf("Shutting down server %s: %v", srvName, err)
		}
	}
	return n.clients.Shutdown()
}

func (n *Node) updateMetricsOnce() {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	metricsRegistry.Gauges.Set("memory_sys", int64(mem.Sys))
	if usage, err := cpuUsage(); err == nil {
		metricsRegistry.Gauges.Set("cpu_usage", int64(usage))
	}
	n.metricsMu.Lock()
	n.metricsSnapshot = n.getSnapshotMetrics()
	metricsRegistry.Counters.UpdateDelta()
	metricsRegistry.HDRHistograms.Rotate()
	n.metricsMu.Unlock()
}

func (n *Node) updateMetrics() {
	for {
		n.RLock()
		interval := n.config.NodeMetricsInterval
		n.RUnlock()
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
		n.RLock()
		interval := n.config.NodePingInterval
		n.RUnlock()
		select {
		case <-n.shutdownCh:
			return
		case <-time.After(interval):
			err := n.pubPing()
			if err != nil {
				logger.CRITICAL.Println(err)
			}
		}
	}
}

func (n *Node) cleanNodeInfo() {
	for {
		n.RLock()
		interval := n.config.NodeInfoCleanInterval
		n.RUnlock()
		select {
		case <-n.shutdownCh:
			return
		case <-time.After(interval):
			n.RLock()
			delay := n.config.NodeInfoMaxDelay
			n.RUnlock()

			n.nodesMu.Lock()
			for uid, info := range n.nodes {
				if time.Now().Unix()-info.Updated() > int64(delay.Seconds()) {
					delete(n.nodes, uid)
				}
			}
			n.nodesMu.Unlock()
		}
	}
}

func (n *Node) Channels() ([]proto.Channel, error) {
	return n.engine.Channels()
}

func (n *Node) Stats() proto.ServerStats {
	n.nodesMu.Lock()
	nodes := make([]proto.NodeInfo, len(n.nodes))
	i := 0
	for _, info := range n.nodes {
		nodes[i] = info
		i++
	}
	n.nodesMu.Unlock()

	n.RLock()
	interval := n.config.NodeMetricsInterval
	n.RUnlock()

	return proto.ServerStats{
		MetricsInterval: int64(interval.Seconds()),
		Nodes:           nodes,
	}
}

func (n *Node) getRawMetrics() map[string]int64 {
	m := make(map[string]int64)
	for name, val := range metricsRegistry.Counters.LoadValues() {
		m[name] = val
	}
	for name, val := range metricsRegistry.HDRHistograms.LoadValues() {
		m[name] = val
	}
	for name, val := range metricsRegistry.Gauges.LoadValues() {
		m[name] = val
	}
	return m
}

func (n *Node) getSnapshotMetrics() map[string]int64 {
	m := make(map[string]int64)
	for name, val := range metricsRegistry.Counters.LoadIntervalValues() {
		m[name] = val
	}
	for name, val := range metricsRegistry.HDRHistograms.LoadValues() {
		m[name] = val
	}
	for name, val := range metricsRegistry.Gauges.LoadValues() {
		m[name] = val
	}
	return m
}

func (n *Node) Node() proto.NodeInfo {
	n.nodesMu.Lock()
	info, ok := n.nodes[n.uid]
	if !ok {
		logger.WARN.Println("node called but no local node info yet, returning garbage")
	}
	n.nodesMu.Unlock()

	info.SetMetrics(n.getRawMetrics())

	return info
}

// ControlMsg handles messages from control channel - control messages used for internal
// communication between nodes to share state or proto.
func (n *Node) ControlMsg(cmd *proto.ControlMessage) error {

	if cmd.UID == n.uid {
		// Sent by this node.
		return nil
	}

	method := cmd.Method
	params := cmd.Params

	switch method {
	case "ping":
		var cmd proto.PingControlCommand
		err := json.Unmarshal(*params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return proto.ErrInvalidMessage
		}
		return n.pingCmd(&cmd)
	case "unsubscribe":
		var cmd proto.UnsubscribeControlCommand
		err := json.Unmarshal(*params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return proto.ErrInvalidMessage
		}
		return n.unsubscribeUser(cmd.User, cmd.Channel)
	case "disconnect":
		var cmd proto.DisconnectControlCommand
		err := json.Unmarshal(*params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return proto.ErrInvalidMessage
		}
		return n.disconnectUser(cmd.User)
	default:
		logger.ERROR.Println("unknown control message method", method)
		return proto.ErrInvalidMessage
	}
}

// AdminMsg handlesadmin message broadcasting it to all admins connected to this node.
func (n *Node) AdminMsg(msg *proto.AdminMessage) error {
	hasAdmins := n.admins.NumAdmins() > 0
	if !hasAdmins {
		return nil
	}
	resp := proto.NewAPIAdminMessageResponse(msg.Params)
	byteMessage, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return n.admins.Broadcast(byteMessage)
}

// ClientMsg handles messages published by web application or client into channel.
// The goal of this method to deliver this message to all clients on this node subscribed
// on channel.
func (n *Node) ClientMsg(ch proto.Channel, msg *proto.Message) error {
	numSubscribers := n.clients.NumSubscribers(ch)
	hasCurrentSubscribers := numSubscribers > 0
	if !hasCurrentSubscribers {
		return nil
	}
	resp := proto.NewClientMessage(msg)
	byteMessage, err := resp.Marshal()
	if err != nil {
		return err
	}
	return n.clients.Broadcast(ch, byteMessage)
}

func makeErrChan(err error) <-chan error {
	ret := make(chan error, 1)
	ret <- err
	return ret
}

// Publish sends a message to all clients subscribed on channel.
func (n *Node) Publish(ch proto.Channel, data []byte, client proto.ConnID, info *proto.ClientInfo) error {
	return <-n.PublishAsync(ch, data, client, info)
}

// PublishAsync sends a message into channel with provided data, client and client info.
func (n *Node) PublishAsync(ch proto.Channel, data []byte, client proto.ConnID, info *proto.ClientInfo) <-chan error {
	if string(ch) == "" || len(data) == 0 {
		return makeErrChan(proto.ErrInvalidMessage)
	}

	chOpts, err := n.ChannelOpts(ch)
	if err != nil {
		return makeErrChan(err)
	}

	return n.pubClient(ch, chOpts, data, client, info)
}

// pubControl publishes message into control channel so all running
// nodes will receive and handle it.
func (n *Node) pubControl(method string, params []byte) error {
	return <-n.engine.PublishControl(proto.NewControlMessage(n.uid, method, params))
}

// pubAdmin publishes message to admins.
func (n *Node) pubAdmin(method string, params []byte) <-chan error {
	return n.engine.PublishAdmin(proto.NewAdminMessage(method, params))
}

// pubClient publishes message into channel so all running nodes
// will receive it and will send to all clients on node subscribed on channel.
func (n *Node) pubClient(ch proto.Channel, chOpts proto.ChannelOptions, data []byte, client proto.ConnID, info *proto.ClientInfo) <-chan error {
	message := proto.NewMessage(ch, data, client, info)
	metricsRegistry.Counters.Inc("num_msg_published")
	if chOpts.Watch {
		byteMessage, err := json.Marshal(message)
		if err != nil {
			logger.ERROR.Println(err)
		} else {
			n.pubAdmin("message", byteMessage)
		}
	}
	return n.engine.PublishMessage(ch, message, &chOpts)
}

// PubJoin allows to publish join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (n *Node) PubJoin(ch proto.Channel, info proto.ClientInfo) error {
	return <-n.engine.PublishJoin(ch, proto.NewJoinMessage(ch, info))
}

// PubLeave allows to publish join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (n *Node) PubLeave(ch proto.Channel, info proto.ClientInfo) error {
	return <-n.engine.PublishLeave(ch, proto.NewLeaveMessage(ch, info))
}

func (n *Node) JoinMsg(ch proto.Channel, msg *proto.JoinMessage) error {
	hasCurrentSubscribers := n.clients.NumSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	resp := proto.NewClientJoinMessage(msg)
	byteMessage, err := resp.Marshal()
	if err != nil {
		return err
	}
	return n.clients.Broadcast(ch, byteMessage)
}

func (n *Node) LeaveMsg(ch proto.Channel, msg *proto.LeaveMessage) error {
	hasCurrentSubscribers := n.clients.NumSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	resp := proto.NewClientLeaveMessage(msg)
	byteMessage, err := resp.Marshal()
	if err != nil {
		return err
	}
	return n.clients.Broadcast(ch, byteMessage)
}

// pubPing sends control ping message to all nodes - this message
// contains information about current node.
func (n *Node) pubPing() error {
	n.RLock()
	metricsRegistry.Gauges.Set("num_clients", int64(n.clients.NumClients()))
	metricsRegistry.Gauges.Set("num_unique_clients", int64(n.clients.NumUniqueClients()))
	metricsRegistry.Gauges.Set("num_channels", int64(n.clients.NumChannels()))
	metricsRegistry.Gauges.Set("num_goroutine", int64(runtime.NumGoroutine()))
	metricsRegistry.Gauges.Set("num_cpu", int64(runtime.NumCPU()))
	metricsRegistry.Gauges.Set("gomaxprocs", int64(runtime.GOMAXPROCS(-1)))

	metricsSnapshot := make(map[string]int64)
	n.metricsMu.RLock()
	for k, v := range n.metricsSnapshot {
		metricsSnapshot[k] = v
	}
	n.metricsMu.RUnlock()

	info := proto.NodeInfo{
		UID:     n.uid,
		Name:    n.config.Name,
		Started: n.started,
		Metrics: metricsSnapshot,
	}
	n.RUnlock()

	cmd := &proto.PingControlCommand{Info: info}

	err := n.pingCmd(cmd)
	if err != nil {
		logger.ERROR.Println(err)
	}

	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	return n.pubControl("ping", cmdBytes)
}

// pubUnsubscribe publishes unsubscribe control message to all nodes – so all
// nodes could unsubscribe user from channel.
func (n *Node) pubUnsubscribe(user proto.UserID, ch proto.Channel) error {

	cmd := &proto.UnsubscribeControlCommand{
		User:    user,
		Channel: ch,
	}

	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	return n.pubControl("unsubscribe", cmdBytes)
}

// pubDisconnect publishes disconnect control message to all nodes – so all
// nodes could disconnect user from Centrifugo.
func (n *Node) pubDisconnect(user proto.UserID) error {

	cmd := &proto.DisconnectControlCommand{
		User: user,
	}

	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	return n.pubControl("disconnect", cmdBytes)
}

// pingCmd handles ping control command i.e. updates information about known nodes.
func (n *Node) pingCmd(cmd *proto.PingControlCommand) error {
	info := cmd.Info
	info.SetUpdated(time.Now().Unix())
	n.nodesMu.Lock()
	n.nodes[info.UID] = info
	n.nodesMu.Unlock()
	return nil
}

// AddConn registers authenticated connection in clientConnectionHub
// this allows to make operations with user connection on demand.
func (n *Node) AddClientConn(c conns.ClientConn) error {
	return n.clients.Add(c)
}

// removeConn removes client connection from connection registry.
func (n *Node) RemoveClientConn(c conns.ClientConn) error {
	return n.clients.Remove(c)
}

// AddClientSub registers subscription of connection on channel in both
// engine and clientSubscriptionHub.
func (n *Node) AddClientSub(ch proto.Channel, c conns.ClientConn) error {
	first, err := n.clients.AddSub(ch, c)
	if err != nil {
		return err
	}
	if first {
		return n.engine.Subscribe(ch)
	}
	return nil
}

// RemoveClientSub removes subscription of connection on channel
// from both engine and clientSubscriptionHub.
func (n *Node) RemoveClientSub(ch proto.Channel, c conns.ClientConn) error {
	empty, err := n.clients.RemoveSub(ch, c)
	if err != nil {
		return err
	}
	if empty {
		return n.engine.Unsubscribe(ch)
	}
	return nil
}

// Unsubscribe unsubscribes user from channel, if channel is equal to empty
// string then user will be unsubscribed from all channels.
func (n *Node) Unsubscribe(user proto.UserID, ch proto.Channel) error {

	if string(user) == "" {
		return proto.ErrInvalidMessage
	}

	if string(ch) != "" {
		_, err := n.ChannelOpts(ch)
		if err != nil {
			return err
		}
	}

	// First unsubscribe on this node.
	err := n.unsubscribeUser(user, ch)
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

// unsubscribeUser unsubscribes user from channel on this node. If channel
// is an empty string then user will be unsubscribed from all channels.
func (n *Node) unsubscribeUser(user proto.UserID, ch proto.Channel) error {
	userConnections := n.clients.UserConnections(user)
	for _, c := range userConnections {
		var channels []proto.Channel
		if string(ch) == "" {
			// unsubscribe from all channels
			channels = c.Channels()
		} else {
			channels = []proto.Channel{ch}
		}

		for _, channel := range channels {
			err := c.Unsubscribe(channel)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Disconnect allows to close all user connections to Centrifugo. Note that user still
// can try to reconnect to the server after being disconnected.
func (n *Node) Disconnect(user proto.UserID) error {

	if string(user) == "" {
		return proto.ErrInvalidMessage
	}

	// first disconnect user from this node
	err := n.disconnectUser(user)
	if err != nil {
		return proto.ErrInternalServerError
	}
	// second send disconnect control message to other nodes
	err = n.pubDisconnect(user)
	if err != nil {
		return proto.ErrInternalServerError
	}
	return nil
}

// disconnectUser closes client connections of user on current node.
func (n *Node) disconnectUser(user proto.UserID) error {
	userConnections := n.clients.UserConnections(user)
	for _, c := range userConnections {
		err := c.Close("disconnect")
		if err != nil {
			return err
		}
	}
	return nil
}

// namespaceKey returns namespace key from channel name if exists.
func (n *Node) namespaceKey(ch proto.Channel) NamespaceKey {
	cTrim := strings.TrimPrefix(string(ch), n.config.PrivateChannelPrefix)
	if strings.Contains(cTrim, n.config.NamespaceChannelBoundary) {
		parts := strings.SplitN(cTrim, n.config.NamespaceChannelBoundary, 2)
		return NamespaceKey(parts[0])
	}
	return NamespaceKey("")
}

// channelOpts returns channel options for channel using current application structure.
func (n *Node) ChannelOpts(ch proto.Channel) (proto.ChannelOptions, error) {
	n.RLock()
	defer n.RUnlock()
	nk := n.namespaceKey(ch)
	return n.config.channelOpts(nk)
}

// addPresence proxies presence adding to engine.
func (n *Node) AddPresence(ch proto.Channel, uid proto.ConnID, info proto.ClientInfo) error {
	n.RLock()
	expire := int(n.config.PresenceExpireInterval.Seconds())
	n.RUnlock()
	return n.engine.AddPresence(ch, uid, info, expire)
}

// RemovePresence proxies presence removing to engine.
func (n *Node) RemovePresence(ch proto.Channel, uid proto.ConnID) error {
	return n.engine.RemovePresence(ch, uid)
}

// Presence returns a map of active clients in project channel.
func (n *Node) Presence(ch proto.Channel) (map[proto.ConnID]proto.ClientInfo, error) {

	if string(ch) == "" {
		return map[proto.ConnID]proto.ClientInfo{}, proto.ErrInvalidMessage
	}

	chOpts, err := n.ChannelOpts(ch)
	if err != nil {
		return map[proto.ConnID]proto.ClientInfo{}, err
	}

	if !chOpts.Presence {
		return map[proto.ConnID]proto.ClientInfo{}, proto.ErrNotAvailable
	}

	presence, err := n.engine.Presence(ch)
	if err != nil {
		logger.ERROR.Println(err)
		return map[proto.ConnID]proto.ClientInfo{}, proto.ErrInternalServerError
	}
	return presence, nil
}

// History returns a slice of last messages published into project channel.
func (n *Node) History(ch proto.Channel) ([]proto.Message, error) {

	if string(ch) == "" {
		return []proto.Message{}, proto.ErrInvalidMessage
	}

	chOpts, err := n.ChannelOpts(ch)
	if err != nil {
		return []proto.Message{}, err
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		return []proto.Message{}, proto.ErrNotAvailable
	}

	history, err := n.engine.History(ch, 0)
	if err != nil {
		logger.ERROR.Println(err)
		return []proto.Message{}, proto.ErrInternalServerError
	}
	return history, nil
}

func (n *Node) LastMessageID(ch proto.Channel) (proto.MessageID, error) {
	history, err := n.engine.History(ch, 1)
	if err != nil {
		return proto.MessageID(""), err
	}
	if len(history) == 0 {
		return proto.MessageID(""), nil
	}
	return proto.MessageID(history[0].UID), nil
}

// PrivateChannel checks if channel private and therefore subscription
// request on it must be properly signed on web application backend.
func (n *Node) PrivateChannel(ch proto.Channel) bool {
	n.RLock()
	defer n.RUnlock()
	return strings.HasPrefix(string(ch), n.config.PrivateChannelPrefix)
}

// UserAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it.
func (n *Node) UserAllowed(ch proto.Channel, user proto.UserID) bool {
	n.RLock()
	defer n.RUnlock()
	if !strings.Contains(string(ch), n.config.UserChannelBoundary) {
		return true
	}
	parts := strings.Split(string(ch), n.config.UserChannelBoundary)
	allowedUsers := strings.Split(parts[len(parts)-1], n.config.UserChannelSeparator)
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
func (n *Node) ClientAllowed(ch proto.Channel, client proto.ConnID) bool {
	n.RLock()
	defer n.RUnlock()
	if !strings.Contains(string(ch), n.config.ClientChannelBoundary) {
		return true
	}
	parts := strings.Split(string(ch), n.config.ClientChannelBoundary)
	allowedClient := parts[len(parts)-1]
	if string(client) == allowedClient {
		return true
	}
	return false
}
