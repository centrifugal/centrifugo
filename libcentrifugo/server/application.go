// Package server is a real-time core for Centrifugo server.
package server

import (
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/metrics"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/satori/go.uuid"
)

// Application is a heart of Centrifugo – it internally manages client and admin hubs,
// maintains information about other Centrifugo nodes, keeps references to
// config, engine, metrics etc.
type Application struct {
	sync.RWMutex

	// unique id for this application (node).
	uid string

	// started is unix time of node start.
	started int64

	// nodes is a map with information about nodes known.
	nodes map[string]proto.NodeInfo

	// nodesMu allows to synchronize access to nodes.
	nodesMu sync.Mutex

	// hub to manage client connections.
	clients *clientHub

	// hub to manage admin connections.
	admins *adminHub

	// engine to use - in memory or redis.
	engine engine.Engine

	// config for application.
	config *config.Config

	// mediator allows integrate libcentrifugo Application with external go code.
	mediator Mediator

	// shutdown is a flag which is only true when application is going to shut down.
	shutdown bool

	// shutdownCh is a channel which is closed when shutdown happens.
	shutdownCh chan struct{}

	// metrics holds various counters and timers different parts of Centrifugo update.
	metrics *metrics.Registry

	// save metrics snapshot until next metrics interval.
	metricsSnapshot map[string]int64

	// protect access to metrics snapshot.
	metricsMu sync.RWMutex
}

func init() {
	metrics.Metrics.RegisterCounter("num_msg_published", metrics.NewCounter())
	metrics.Metrics.RegisterCounter("num_msg_queued", metrics.NewCounter())
	metrics.Metrics.RegisterCounter("num_msg_sent", metrics.NewCounter())
	metrics.Metrics.RegisterCounter("num_api_requests", metrics.NewCounter())
	metrics.Metrics.RegisterCounter("num_client_requests", metrics.NewCounter())
	metrics.Metrics.RegisterCounter("bytes_client_in", metrics.NewCounter())
	metrics.Metrics.RegisterCounter("bytes_client_out", metrics.NewCounter())
	metrics.Metrics.RegisterCounter("num_msg_published", metrics.NewCounter())

	metrics.Metrics.RegisterGauge("memory_sys", metrics.NewGauge())
	metrics.Metrics.RegisterGauge("cpu_usage", metrics.NewGauge())
	metrics.Metrics.RegisterGauge("num_goroutine", metrics.NewGauge())
	metrics.Metrics.RegisterGauge("num_clients", metrics.NewGauge())
	metrics.Metrics.RegisterGauge("num_unique_clients", metrics.NewGauge())
	metrics.Metrics.RegisterGauge("num_channels", metrics.NewGauge())
	metrics.Metrics.RegisterGauge("gomaxprocs", metrics.NewGauge())
	metrics.Metrics.RegisterGauge("num_cpu", metrics.NewGauge())

	quantiles := []float64{50, 90, 99, 99.99}
	var minValue int64 = 1        // record latencies in microseconds, min resolution 1mks.
	var maxValue int64 = 60000000 // record latencies in microseconds, max resolution 60s.
	numBuckets := 15              // histograms will be rotated every time we updating snapshot.
	sigfigs := 3
	metrics.Metrics.RegisterHDRHistogram("http_api", metrics.NewHDRHistogram(numBuckets, minValue, maxValue, sigfigs, quantiles, "microseconds"))
	metrics.Metrics.RegisterHDRHistogram("client_api", metrics.NewHDRHistogram(numBuckets, minValue, maxValue, sigfigs, quantiles, "microseconds"))
}

// New returns new server instance backed by Application, the only required
// argument is config. Engine must be set via corresponding methods.
func New(c *config.Config) Server {
	app := &Application{
		uid:             uuid.NewV4().String(),
		config:          c,
		clients:         newClientHub(),
		admins:          newAdminHub(),
		nodes:           make(map[string]proto.NodeInfo),
		started:         time.Now().Unix(),
		metrics:         metrics.Metrics,
		metricsSnapshot: make(map[string]int64),
		shutdownCh:      make(chan struct{}),
	}

	// Create initial snapshot with empty values.
	app.metricsMu.Lock()
	app.metricsSnapshot = app.getSnapshotMetrics()
	app.metricsMu.Unlock()

	return app
}

func (app *Application) Config() config.Config {
	app.RLock()
	c := *app.config
	app.RUnlock()
	return c
}

func (app *Application) NotifyShutdown() chan struct{} {
	return app.shutdownCh
}

// Run performs all startup actions. At moment must be called once on start
// after engine and structure set.
func (app *Application) Run() error {
	if err := app.engine.Run(); err != nil {
		return err
	}
	go app.sendNodePingMsg()
	go app.cleanNodeInfo()
	go app.updateMetrics()

	app.RLock()
	if app.config.Insecure {
		logger.WARN.Println("Running in INSECURE client mode")
	}
	if app.config.InsecureAPI {
		logger.WARN.Println("Running in INSECURE API mode")
	}
	if app.config.InsecureAdmin {
		logger.WARN.Println("Running in INSECURE admin mode")
	}
	if app.config.Debug {
		logger.WARN.Println("Running in DEBUG mode")
	}
	app.RUnlock()

	return app.runHTTPServer()
}

// Shutdown sets shutdown flag and does various connection clean ups (at moment only unsubscribes
// all clients from all channels and disconnects them).
func (app *Application) Shutdown() error {
	app.Lock()
	if app.shutdown {
		app.Unlock()
		return nil
	}
	app.shutdown = true
	close(app.shutdownCh)
	app.Unlock()
	app.clients.shutdown()
	return nil
}

func (app *Application) Channels() []proto.Channel {
	return app.clients.channels()
}

func (app *Application) NumSubscribers(ch proto.Channel) int {
	return app.clients.numSubscribers(ch)
}

func (app *Application) updateMetricsOnce() {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	app.metrics.Gauges.Set("memory_sys", int64(mem.Sys))
	if usage, err := cpuUsage(); err == nil {
		app.metrics.Gauges.Set("cpu_usage", int64(usage))
	}
	app.metricsMu.Lock()
	app.metricsSnapshot = app.getSnapshotMetrics()
	app.metrics.Counters.UpdateDelta()
	app.metrics.HDRHistograms.Rotate()
	app.metricsMu.Unlock()
}

func (app *Application) updateMetrics() {
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

func (app *Application) sendNodePingMsg() {
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

func (app *Application) cleanNodeInfo() {
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
func (app *Application) SetConfig(c *config.Config) {
	app.Lock()
	defer app.Unlock()
	app.config = c
	if app.config.Insecure {
		logger.WARN.Println("libcentrifugo: application in INSECURE MODE")
	}
}

// SetEngine binds engine to application.
func (app *Application) SetEngine(e engine.Engine) {
	app.Lock()
	defer app.Unlock()
	app.engine = e
}

// SetMediator binds mediator to application.
func (app *Application) SetMediator(m Mediator) {
	app.Lock()
	defer app.Unlock()
	app.mediator = m
}

func (app *Application) channels() ([]proto.Channel, error) {
	return app.engine.Channels()
}

func (app *Application) stats() proto.ServerStats {
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

func (app *Application) getRawMetrics() map[string]int64 {
	m := make(map[string]int64)
	for name, val := range app.metrics.Counters.LoadValues() {
		m[name] = val
	}
	for name, val := range app.metrics.HDRHistograms.LoadValues() {
		m[name] = val
	}
	for name, val := range app.metrics.Gauges.LoadValues() {
		m[name] = val
	}
	return m
}

func (app *Application) getSnapshotMetrics() map[string]int64 {
	m := make(map[string]int64)
	for name, val := range app.metrics.Counters.LoadIntervalValues() {
		m[name] = val
	}
	for name, val := range app.metrics.HDRHistograms.LoadValues() {
		m[name] = val
	}
	for name, val := range app.metrics.Gauges.LoadValues() {
		m[name] = val
	}
	return m
}

func (app *Application) node() proto.NodeInfo {
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
func (app *Application) ControlMsg(cmd *proto.ControlMessage) error {

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
func (app *Application) AdminMsg(msg *proto.AdminMessage) error {
	app.admins.RLock()
	hasAdmins := len(app.admins.connections) > 0
	app.admins.RUnlock()
	if !hasAdmins {
		return nil
	}
	resp := proto.NewAPIAdminMessageResponse(msg.Params)
	byteMessage, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return app.admins.broadcast(byteMessage)
}

// ClientMsg handles messages published by web application or client into channel.
// The goal of this method to deliver this message to all clients on this node subscribed
// on channel.
func (app *Application) ClientMsg(ch proto.Channel, msg *proto.Message) error {
	numSubscribers := app.clients.numSubscribers(ch)
	hasCurrentSubscribers := numSubscribers > 0
	if !hasCurrentSubscribers {
		return nil
	}
	resp := proto.NewClientMessage()
	resp.Body = *msg
	byteMessage, err := resp.Marshal()
	if err != nil {
		return err
	}
	return app.clients.broadcast(ch, byteMessage)
}

// Publish sends a message to all clients subscribed on channel with provided data, client and ClientInfo.
func (app *Application) Publish(ch proto.Channel, data []byte, client proto.ConnID, info *proto.ClientInfo) error {

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
func (app *Application) publishAsync(ch proto.Channel, data []byte, client proto.ConnID, info *proto.ClientInfo, fromClient bool) <-chan error {
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
func (app *Application) publish(ch proto.Channel, data []byte, client proto.ConnID, info *proto.ClientInfo, fromClient bool) error {
	return <-app.publishAsync(ch, data, client, info, fromClient)
}

// pubControl publishes message into control channel so all running
// nodes will receive and handle it.
func (app *Application) pubControl(method string, params []byte) error {
	return <-app.engine.PublishControl(proto.NewControlMessage(app.uid, method, params))
}

// pubAdmin publishes message to admins.
func (app *Application) pubAdmin(method string, params []byte) <-chan error {
	return app.engine.PublishAdmin(proto.NewAdminMessage(method, params))
}

// pubClient publishes message into channel so all running nodes
// will receive it and will send to all clients on node subscribed on channel.
func (app *Application) pubClient(ch proto.Channel, chOpts config.ChannelOptions, data []byte, client proto.ConnID, info *proto.ClientInfo) <-chan error {
	message := proto.NewMessage(ch, data, client, info)
	app.metrics.Counters.Inc("num_msg_published")
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
func (app *Application) pubJoin(ch proto.Channel, info proto.ClientInfo) error {
	return <-app.engine.PublishJoin(ch, proto.NewJoinMessage(ch, info))
}

// pubLeave allows to publish join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (app *Application) pubLeave(ch proto.Channel, info proto.ClientInfo) error {
	return <-app.engine.PublishLeave(ch, proto.NewLeaveMessage(ch, info))
}

func (app *Application) JoinMsg(ch proto.Channel, msg *proto.JoinMessage) error {
	hasCurrentSubscribers := app.clients.numSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	resp := proto.NewClientJoinMessage()
	resp.Body = *msg
	byteMessage, err := resp.Marshal()
	if err != nil {
		return err
	}
	return app.clients.broadcast(ch, byteMessage)
}

func (app *Application) LeaveMsg(ch proto.Channel, msg *proto.LeaveMessage) error {
	hasCurrentSubscribers := app.clients.numSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	resp := proto.NewClientLeaveMessage()
	resp.Body = *msg
	byteMessage, err := resp.Marshal()
	if err != nil {
		return err
	}
	return app.clients.broadcast(ch, byteMessage)
}

// pubPing sends control ping message to all nodes - this message
// contains information about current node.
func (app *Application) pubPing() error {
	app.RLock()
	app.metrics.Gauges.Set("num_clients", int64(app.nClients()))
	app.metrics.Gauges.Set("num_unique_clients", int64(app.nUniqueClients()))
	app.metrics.Gauges.Set("num_channels", int64(app.nChannels()))
	app.metrics.Gauges.Set("num_goroutine", int64(runtime.NumGoroutine()))
	app.metrics.Gauges.Set("num_cpu", int64(runtime.NumCPU()))
	app.metrics.Gauges.Set("gomaxprocs", int64(runtime.GOMAXPROCS(-1)))

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
func (app *Application) pubUnsubscribe(user proto.UserID, ch proto.Channel) error {

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
func (app *Application) pubDisconnect(user proto.UserID) error {

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
func (app *Application) pingCmd(cmd *proto.PingControlCommand) error {
	info := cmd.Info
	info.SetUpdated(time.Now().Unix())
	app.nodesMu.Lock()
	app.nodes[info.UID] = info
	app.nodesMu.Unlock()
	return nil
}

// addConn registers authenticated connection in clientConnectionHub
// this allows to make operations with user connection on demand.
func (app *Application) addConn(c clientConn) error {
	return app.clients.add(c)
}

// removeConn removes client connection from connection registry.
func (app *Application) removeConn(c clientConn) error {
	return app.clients.remove(c)
}

// addSub registers subscription of connection on channel in both
// engine and clientSubscriptionHub.
func (app *Application) addSub(ch proto.Channel, c clientConn) error {
	first, err := app.clients.addSub(ch, c)
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
func (app *Application) removeSub(ch proto.Channel, c clientConn) error {
	empty, err := app.clients.removeSub(ch, c)
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
func (app *Application) Unsubscribe(user proto.UserID, ch proto.Channel) error {

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
func (app *Application) unsubscribeUser(user proto.UserID, ch proto.Channel) error {
	userConnections := app.clients.userConnections(user)
	for _, c := range userConnections {
		var channels []proto.Channel
		if string(ch) == "" {
			// unsubscribe from all channels
			channels = c.channels()
		} else {
			channels = []proto.Channel{ch}
		}

		for _, channel := range channels {
			err := c.unsubscribe(channel)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Disconnect allows to close all user connections to Centrifugo. Note that user still
// can try to reconnect to the server after being disconnected.
func (app *Application) Disconnect(user proto.UserID) error {

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
func (app *Application) disconnectUser(user proto.UserID) error {
	userConnections := app.clients.userConnections(user)
	for _, c := range userConnections {
		err := c.close("disconnect")
		if err != nil {
			return err
		}
	}
	return nil
}

// namespaceKey returns namespace key from channel name if exists.
func (app *Application) namespaceKey(ch proto.Channel) config.NamespaceKey {
	cTrim := strings.TrimPrefix(string(ch), app.config.PrivateChannelPrefix)
	if strings.Contains(cTrim, app.config.NamespaceChannelBoundary) {
		parts := strings.SplitN(cTrim, app.config.NamespaceChannelBoundary, 2)
		return config.NamespaceKey(parts[0])
	}
	return config.NamespaceKey("")
}

// channelOpts returns channel options for channel using current application structure.
func (app *Application) channelOpts(ch proto.Channel) (config.ChannelOptions, error) {
	app.RLock()
	defer app.RUnlock()
	nk := app.namespaceKey(ch)
	found, opts := app.config.ChannelOpts(nk)
	if found {
		return opts, nil
	}
	return config.ChannelOptions{}, ErrNamespaceNotFound
}

// addPresence proxies presence adding to engine.
func (app *Application) addPresence(ch proto.Channel, uid proto.ConnID, info proto.ClientInfo) error {
	return app.engine.AddPresence(ch, uid, info)
}

// removePresence proxies presence removing to engine.
func (app *Application) removePresence(ch proto.Channel, uid proto.ConnID) error {
	return app.engine.RemovePresence(ch, uid)
}

// Presence returns a map of active clients in project channel.
func (app *Application) Presence(ch proto.Channel) (map[proto.ConnID]proto.ClientInfo, error) {

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
func (app *Application) History(ch proto.Channel) ([]proto.Message, error) {

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

func (app *Application) lastMessageID(ch proto.Channel) (proto.MessageID, error) {
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
func (app *Application) privateChannel(ch proto.Channel) bool {
	app.RLock()
	defer app.RUnlock()
	return strings.HasPrefix(string(ch), app.config.PrivateChannelPrefix)
}

// userAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it.
func (app *Application) userAllowed(ch proto.Channel, user proto.UserID) bool {
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
func (app *Application) clientAllowed(ch proto.Channel, client proto.ConnID) bool {
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

// addAdminConn registers an admin connection in adminConnectionHub.
func (app *Application) addAdminConn(c adminConn) error {
	return app.admins.add(c)
}

// removeAdminConn admin connection from adminConnectionHub.
func (app *Application) removeAdminConn(c adminConn) error {
	return app.admins.remove(c)
}

// nChannels returns total amount of active channels on this node.
func (app *Application) nChannels() int {
	return app.clients.nChannels()
}

// nClients returns total amount of client connections to this node.
func (app *Application) nClients() int {
	return app.clients.nClients()
}

// nUniqueClients returns total amount of unique client connections to this node.
func (app *Application) nUniqueClients() int {
	return app.clients.nUniqueClients()
}
