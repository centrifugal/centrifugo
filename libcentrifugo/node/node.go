// Package node is a real-time core for Centrifugo server.
package node

import (
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/FZambia/go-logger"
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

	// unique id for this node.
	uid string

	// started is unix time of node start.
	started int64

	// nodes is a map with information about nodes known.
	nodes map[string]proto.NodeInfo

	// nodesMu allows to synchronize access to nodes.
	nodesMu sync.Mutex

	// hub to manage client connections.
	clients ClientHub

	// hub to manage admin connections.
	admins AdminHub

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
	metricsRegistry = metrics.Metrics

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
func New(c *Config) *Node {
	app := &Node{
		uid:             uuid.NewV4().String(),
		config:          c,
		clients:         newClientHub(),
		admins:          newAdminHub(),
		nodes:           make(map[string]proto.NodeInfo),
		started:         time.Now().Unix(),
		metricsSnapshot: make(map[string]int64),
		shutdownCh:      make(chan struct{}),
	}

	// Create initial snapshot with empty values.
	app.metricsMu.Lock()
	app.metricsSnapshot = app.getSnapshotMetrics()
	app.metricsMu.Unlock()

	return app
}

// Config returns a copy of node Config.
func (app *Node) Config() Config {
	app.RLock()
	c := *app.config
	app.RUnlock()
	return c
}

// Notify shutdown returns a channel which will be closed on node shutdown.
func (app *Node) NotifyShutdown() chan struct{} {
	return app.shutdownCh
}

// Run performs all startup actions. At moment must be called once on start
// after engine and structure set.
func (app *Node) Run(opts *RunOptions) error {
	app.Lock()
	app.engine = opts.Engine
	app.servers = opts.Servers
	app.mediator = opts.Mediator
	app.Unlock()

	if err := app.engine.Run(); err != nil {
		return err
	}
	go app.sendNodePingMsg()
	go app.cleanNodeInfo()
	go app.updateMetrics()

	config := app.Config()
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

	return app.runServers()
}

func (app *Node) runServers() error {
	var wg sync.WaitGroup
	for srvName, srv := range app.servers {
		wg.Add(1)
		logger.DEBUG.Printf("Starting %s server", srvName)
		go srv.Run()
		go func() {
			defer wg.Done()
			<-app.shutdownCh
			if err := srv.Shutdown(); err != nil {
				logger.ERROR.Println(err)
			}
		}()
	}
	wg.Wait()
	return nil
}

// Shutdown sets shutdown flag and does various clean ups.
func (app *Node) Shutdown() error {
	app.Lock()
	if app.shutdown {
		app.Unlock()
		return nil
	}
	app.shutdown = true
	close(app.shutdownCh)
	app.Unlock()
	app.clients.Shutdown()
	return nil
}

func (app *Node) ClientHub() ClientHub {
	return app.clients
}

func (app *Node) AdminHub() AdminHub {
	return app.admins
}

func (app *Node) updateMetricsOnce() {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	metricsRegistry.Gauges.Set("memory_sys", int64(mem.Sys))
	if usage, err := cpuUsage(); err == nil {
		metricsRegistry.Gauges.Set("cpu_usage", int64(usage))
	}
	app.metricsMu.Lock()
	app.metricsSnapshot = app.getSnapshotMetrics()
	metricsRegistry.Counters.UpdateDelta()
	metricsRegistry.HDRHistograms.Rotate()
	app.metricsMu.Unlock()
}

func (app *Node) updateMetrics() {
	for {
		app.RLock()
		interval := app.config.NodeMetricsInterval
		app.RUnlock()
		select {
		case <-app.shutdownCh:
			return
		case <-time.After(interval):
			app.updateMetricsOnce()
		}
	}
}

func (app *Node) sendNodePingMsg() {
	err := app.pubPing()
	if err != nil {
		logger.CRITICAL.Println(err)
	}
	for {
		app.RLock()
		interval := app.config.NodePingInterval
		app.RUnlock()
		select {
		case <-app.shutdownCh:
			return
		case <-time.After(interval):
			err := app.pubPing()
			if err != nil {
				logger.CRITICAL.Println(err)
			}
		}
	}
}

func (app *Node) cleanNodeInfo() {
	for {
		app.RLock()
		interval := app.config.NodeInfoCleanInterval
		app.RUnlock()
		select {
		case <-app.shutdownCh:
			return
		case <-time.After(interval):
			app.RLock()
			delay := app.config.NodeInfoMaxDelay
			app.RUnlock()

			app.nodesMu.Lock()
			for uid, info := range app.nodes {
				if time.Now().Unix()-info.Updated() > int64(delay.Seconds()) {
					delete(app.nodes, uid)
				}
			}
			app.nodesMu.Unlock()
		}
	}
}

// SetConfig binds config to application.
func (app *Node) SetConfig(c *Config) {
	app.Lock()
	defer app.Unlock()
	app.config = c
}

func (app *Node) channels() ([]proto.Channel, error) {
	return app.engine.Channels()
}

func (app *Node) stats() proto.ServerStats {
	app.nodesMu.Lock()
	nodes := make([]proto.NodeInfo, len(app.nodes))
	i := 0
	for _, info := range app.nodes {
		nodes[i] = info
		i++
	}
	app.nodesMu.Unlock()

	app.RLock()
	interval := app.config.NodeMetricsInterval
	app.RUnlock()

	return proto.ServerStats{
		MetricsInterval: int64(interval.Seconds()),
		Nodes:           nodes,
	}
}

func (app *Node) getRawMetrics() map[string]int64 {
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

func (app *Node) getSnapshotMetrics() map[string]int64 {
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

func (app *Node) node() proto.NodeInfo {
	app.nodesMu.Lock()
	info, ok := app.nodes[app.uid]
	if !ok {
		logger.WARN.Println("node called but no local node info yet, returning garbage")
	}
	app.nodesMu.Unlock()

	info.SetMetrics(app.getRawMetrics())

	return info
}

// ControlMsg handles messages from control channel - control messages used for internal
// communication between nodes to share state or proto.
func (app *Node) ControlMsg(cmd *proto.ControlMessage) error {

	if cmd.UID == app.uid {
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
			return ErrInvalidMessage
		}
		return app.pingCmd(&cmd)
	case "unsubscribe":
		var cmd proto.UnsubscribeControlCommand
		err := json.Unmarshal(*params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return ErrInvalidMessage
		}
		return app.unsubscribeUser(cmd.User, cmd.Channel)
	case "disconnect":
		var cmd proto.DisconnectControlCommand
		err := json.Unmarshal(*params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return ErrInvalidMessage
		}
		return app.disconnectUser(cmd.User)
	default:
		logger.ERROR.Println("unknown control message method", method)
		return ErrInvalidMessage
	}
}

// AdminMsg handlesadmin message broadcasting it to all admins connected to this node.
func (app *Node) AdminMsg(msg *proto.AdminMessage) error {
	hasAdmins := app.admins.NumAdmins() > 0
	if !hasAdmins {
		return nil
	}
	resp := proto.NewAPIAdminMessageResponse(msg.Params)
	byteMessage, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return app.admins.Broadcast(byteMessage)
}

// ClientMsg handles messages published by web application or client into channel.
// The goal of this method to deliver this message to all clients on this node subscribed
// on channel.
func (app *Node) ClientMsg(ch proto.Channel, msg *proto.Message) error {
	numSubscribers := app.clients.NumSubscribers(ch)
	hasCurrentSubscribers := numSubscribers > 0
	if !hasCurrentSubscribers {
		return nil
	}
	resp := proto.NewClientMessage(msg)
	byteMessage, err := resp.Marshal()
	if err != nil {
		return err
	}
	return app.clients.Broadcast(ch, byteMessage)
}

// Publish sends a message to all clients subscribed on channel.
func (app *Node) Publish(ch proto.Channel, data []byte, client proto.ConnID, info *proto.ClientInfo) error {

	if string(ch) == "" || len(data) == 0 {
		return ErrInvalidMessage
	}

	chOpts, err := app.channelOpts(ch)
	if err != nil {
		return err
	}

	errCh := app.pubClient(ch, chOpts, data, client, info)
	err = <-errCh
	if err != nil {
		logger.ERROR.Println(err)
		return ErrInternalServerError
	}

	return nil
}

func makeErrChan(err error) <-chan error {
	ret := make(chan error, 1)
	ret <- err
	return ret
}

// publish sends a message into channel with provided data, client and client info.
// If fromClient argument is true then internally this method will check client permission to
// publish into this channel.
func (app *Node) publishAsync(ch proto.Channel, data []byte, client proto.ConnID, info *proto.ClientInfo, fromClient bool) <-chan error {
	if string(ch) == "" || len(data) == 0 {
		return makeErrChan(ErrInvalidMessage)
	}

	chOpts, err := app.channelOpts(ch)
	if err != nil {
		return makeErrChan(err)
	}

	app.RLock()
	insecure := app.config.Insecure
	app.RUnlock()

	if fromClient && !chOpts.Publish && !insecure {
		return makeErrChan(ErrPermissionDenied)
	}

	if app.mediator != nil {
		// If mediator is set then we don't need to publish message
		// immediately as mediator will decide itself what to do with it.
		pass := app.mediator.Message(ch, data, client, info)
		if !pass {
			return makeErrChan(nil)
		}
	}

	return app.pubClient(ch, chOpts, data, client, info)
}

// publish sends a message into channel with provided data, client and client info.
// If fromClient argument is true then internally this method will check client permission to
// publish into this channel.
func (app *Node) publish(ch proto.Channel, data []byte, client proto.ConnID, info *proto.ClientInfo, fromClient bool) error {
	return <-app.publishAsync(ch, data, client, info, fromClient)
}

// pubControl publishes message into control channel so all running
// nodes will receive and handle it.
func (app *Node) pubControl(method string, params []byte) error {
	return <-app.engine.PublishControl(proto.NewControlMessage(app.uid, method, params))
}

// pubAdmin publishes message to admins.
func (app *Node) pubAdmin(method string, params []byte) <-chan error {
	return app.engine.PublishAdmin(proto.NewAdminMessage(method, params))
}

// pubClient publishes message into channel so all running nodes
// will receive it and will send to all clients on node subscribed on channel.
func (app *Node) pubClient(ch proto.Channel, chOpts proto.ChannelOptions, data []byte, client proto.ConnID, info *proto.ClientInfo) <-chan error {
	message := proto.NewMessage(ch, data, client, info)
	metricsRegistry.Counters.Inc("num_msg_published")
	if chOpts.Watch {
		byteMessage, err := json.Marshal(message)
		if err != nil {
			logger.ERROR.Println(err)
		} else {
			app.pubAdmin("message", byteMessage)
		}
	}
	return app.engine.PublishMessage(ch, message, &chOpts)
}

// pubJoin allows to publish join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (app *Node) pubJoin(ch proto.Channel, info proto.ClientInfo) error {
	return <-app.engine.PublishJoin(ch, proto.NewJoinMessage(ch, info))
}

// pubLeave allows to publish join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (app *Node) pubLeave(ch proto.Channel, info proto.ClientInfo) error {
	return <-app.engine.PublishLeave(ch, proto.NewLeaveMessage(ch, info))
}

func (app *Node) JoinMsg(ch proto.Channel, msg *proto.JoinMessage) error {
	hasCurrentSubscribers := app.clients.NumSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	resp := proto.NewClientJoinMessage(msg)
	byteMessage, err := resp.Marshal()
	if err != nil {
		return err
	}
	return app.clients.Broadcast(ch, byteMessage)
}

func (app *Node) LeaveMsg(ch proto.Channel, msg *proto.LeaveMessage) error {
	hasCurrentSubscribers := app.clients.NumSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	resp := proto.NewClientLeaveMessage(msg)
	byteMessage, err := resp.Marshal()
	if err != nil {
		return err
	}
	return app.clients.Broadcast(ch, byteMessage)
}

// pubPing sends control ping message to all nodes - this message
// contains information about current node.
func (app *Node) pubPing() error {
	app.RLock()
	metricsRegistry.Gauges.Set("num_clients", int64(app.ClientHub().NumClients()))
	metricsRegistry.Gauges.Set("num_unique_clients", int64(app.ClientHub().NumUniqueClients()))
	metricsRegistry.Gauges.Set("num_channels", int64(app.ClientHub().NumChannels()))
	metricsRegistry.Gauges.Set("num_goroutine", int64(runtime.NumGoroutine()))
	metricsRegistry.Gauges.Set("num_cpu", int64(runtime.NumCPU()))
	metricsRegistry.Gauges.Set("gomaxprocs", int64(runtime.GOMAXPROCS(-1)))

	metricsSnapshot := make(map[string]int64)
	app.metricsMu.RLock()
	for k, v := range app.metricsSnapshot {
		metricsSnapshot[k] = v
	}
	app.metricsMu.RUnlock()

	info := proto.NodeInfo{
		UID:     app.uid,
		Name:    app.config.Name,
		Started: app.started,
		Metrics: metricsSnapshot,
	}
	app.RUnlock()

	cmd := &proto.PingControlCommand{Info: info}

	err := app.pingCmd(cmd)
	if err != nil {
		logger.ERROR.Println(err)
	}

	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	return app.pubControl("ping", cmdBytes)
}

// pubUnsubscribe publishes unsubscribe control message to all nodes – so all
// nodes could unsubscribe user from channel.
func (app *Node) pubUnsubscribe(user proto.UserID, ch proto.Channel) error {

	cmd := &proto.UnsubscribeControlCommand{
		User:    user,
		Channel: ch,
	}

	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	return app.pubControl("unsubscribe", cmdBytes)
}

// pubDisconnect publishes disconnect control message to all nodes – so all
// nodes could disconnect user from Centrifugo.
func (app *Node) pubDisconnect(user proto.UserID) error {

	cmd := &proto.DisconnectControlCommand{
		User: user,
	}

	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	return app.pubControl("disconnect", cmdBytes)
}

// pingCmd handles ping control command i.e. updates information about known nodes.
func (app *Node) pingCmd(cmd *proto.PingControlCommand) error {
	info := cmd.Info
	info.SetUpdated(time.Now().Unix())
	app.nodesMu.Lock()
	app.nodes[info.UID] = info
	app.nodesMu.Unlock()
	return nil
}

// addConn registers authenticated connection in clientConnectionHub
// this allows to make operations with user connection on demand.
func (app *Node) addConn(c ClientConn) error {
	return app.clients.Add(c)
}

// removeConn removes client connection from connection registry.
func (app *Node) removeConn(c ClientConn) error {
	return app.clients.Remove(c)
}

// addSub registers subscription of connection on channel in both
// engine and clientSubscriptionHub.
func (app *Node) addSub(ch proto.Channel, c ClientConn) error {
	first, err := app.clients.AddSub(ch, c)
	if err != nil {
		return err
	}
	if first {
		return app.engine.Subscribe(ch)
	}
	return nil
}

// removeSub removes subscription of connection on channel
// from both engine and clientSubscriptionHub.
func (app *Node) removeSub(ch proto.Channel, c ClientConn) error {
	empty, err := app.clients.RemoveSub(ch, c)
	if err != nil {
		return err
	}
	if empty {
		return app.engine.Unsubscribe(ch)
	}
	return nil
}

// Unsubscribe unsubscribes user from channel, if channel is equal to empty
// string then user will be unsubscribed from all channels.
func (app *Node) Unsubscribe(user proto.UserID, ch proto.Channel) error {

	if string(user) == "" {
		return ErrInvalidMessage
	}

	if string(ch) != "" {
		_, err := app.channelOpts(ch)
		if err != nil {
			return err
		}
	}

	// First unsubscribe on this node.
	err := app.unsubscribeUser(user, ch)
	if err != nil {
		return ErrInternalServerError
	}
	// Second send unsubscribe control message to other nodes.
	err = app.pubUnsubscribe(user, ch)
	if err != nil {
		return ErrInternalServerError
	}
	return nil
}

// unsubscribeUser unsubscribes user from channel on this node. If channel
// is an empty string then user will be unsubscribed from all channels.
func (app *Node) unsubscribeUser(user proto.UserID, ch proto.Channel) error {
	userConnections := app.clients.UserConnections(user)
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
func (app *Node) Disconnect(user proto.UserID) error {

	if string(user) == "" {
		return ErrInvalidMessage
	}

	// first disconnect user from this node
	err := app.disconnectUser(user)
	if err != nil {
		return ErrInternalServerError
	}
	// second send disconnect control message to other nodes
	err = app.pubDisconnect(user)
	if err != nil {
		return ErrInternalServerError
	}
	return nil
}

// disconnectUser closes client connections of user on current node.
func (app *Node) disconnectUser(user proto.UserID) error {
	userConnections := app.clients.UserConnections(user)
	for _, c := range userConnections {
		err := c.Close("disconnect")
		if err != nil {
			return err
		}
	}
	return nil
}

// namespaceKey returns namespace key from channel name if exists.
func (app *Node) namespaceKey(ch proto.Channel) NamespaceKey {
	cTrim := strings.TrimPrefix(string(ch), app.config.PrivateChannelPrefix)
	if strings.Contains(cTrim, app.config.NamespaceChannelBoundary) {
		parts := strings.SplitN(cTrim, app.config.NamespaceChannelBoundary, 2)
		return NamespaceKey(parts[0])
	}
	return NamespaceKey("")
}

// channelOpts returns channel options for channel using current application structure.
func (app *Node) channelOpts(ch proto.Channel) (proto.ChannelOptions, error) {
	app.RLock()
	defer app.RUnlock()
	nk := app.namespaceKey(ch)
	found, opts := app.config.ChannelOpts(nk)
	if found {
		return opts, nil
	}
	return proto.ChannelOptions{}, ErrNamespaceNotFound
}

// addPresence proxies presence adding to engine.
func (app *Node) addPresence(ch proto.Channel, uid proto.ConnID, info proto.ClientInfo) error {
	app.RLock()
	expire := int(app.config.PresenceExpireInterval.Seconds())
	app.RUnlock()
	return app.engine.AddPresence(ch, uid, info, expire)
}

// removePresence proxies presence removing to engine.
func (app *Node) removePresence(ch proto.Channel, uid proto.ConnID) error {
	return app.engine.RemovePresence(ch, uid)
}

// Presence returns a map of active clients in project channel.
func (app *Node) Presence(ch proto.Channel) (map[proto.ConnID]proto.ClientInfo, error) {

	if string(ch) == "" {
		return map[proto.ConnID]proto.ClientInfo{}, ErrInvalidMessage
	}

	chOpts, err := app.channelOpts(ch)
	if err != nil {
		return map[proto.ConnID]proto.ClientInfo{}, err
	}

	if !chOpts.Presence {
		return map[proto.ConnID]proto.ClientInfo{}, ErrNotAvailable
	}

	presence, err := app.engine.Presence(ch)
	if err != nil {
		logger.ERROR.Println(err)
		return map[proto.ConnID]proto.ClientInfo{}, ErrInternalServerError
	}
	return presence, nil
}

// History returns a slice of last messages published into project channel.
func (app *Node) History(ch proto.Channel) ([]proto.Message, error) {

	if string(ch) == "" {
		return []proto.Message{}, ErrInvalidMessage
	}

	chOpts, err := app.channelOpts(ch)
	if err != nil {
		return []proto.Message{}, err
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		return []proto.Message{}, ErrNotAvailable
	}

	history, err := app.engine.History(ch, 0)
	if err != nil {
		logger.ERROR.Println(err)
		return []proto.Message{}, ErrInternalServerError
	}
	return history, nil
}

func (app *Node) lastMessageID(ch proto.Channel) (proto.MessageID, error) {
	history, err := app.engine.History(ch, 1)
	if err != nil {
		return proto.MessageID(""), err
	}
	if len(history) == 0 {
		return proto.MessageID(""), nil
	}
	return proto.MessageID(history[0].UID), nil
}

// privateChannel checks if channel private and therefore subscription
// request on it must be properly signed on web application backend.
func (app *Node) privateChannel(ch proto.Channel) bool {
	app.RLock()
	defer app.RUnlock()
	return strings.HasPrefix(string(ch), app.config.PrivateChannelPrefix)
}

// userAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it.
func (app *Node) userAllowed(ch proto.Channel, user proto.UserID) bool {
	app.RLock()
	defer app.RUnlock()
	if !strings.Contains(string(ch), app.config.UserChannelBoundary) {
		return true
	}
	parts := strings.Split(string(ch), app.config.UserChannelBoundary)
	allowedUsers := strings.Split(parts[len(parts)-1], app.config.UserChannelSeparator)
	for _, allowedUser := range allowedUsers {
		if string(user) == allowedUser {
			return true
		}
	}
	return false
}

// clientAllowed checks if client can subscribe on channel - as channel
// can contain special part in the end to indicate which client allowed
// to subscribe on it.
func (app *Node) clientAllowed(ch proto.Channel, client proto.ConnID) bool {
	app.RLock()
	defer app.RUnlock()
	if !strings.Contains(string(ch), app.config.ClientChannelBoundary) {
		return true
	}
	parts := strings.Split(string(ch), app.config.ClientChannelBoundary)
	allowedClient := parts[len(parts)-1]
	if string(client) == allowedClient {
		return true
	}
	return false
}
