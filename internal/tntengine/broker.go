package tntengine

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/centrifugal/centrifugo/v4/internal/tools"

	"github.com/FZambia/tarantool"
	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

const internalChannelPrefix = "__"

const (
	// tarantoolControlChannel is a name for control channel.
	tarantoolControlChannel = internalChannelPrefix + "control"
	// tarantoolNodeChannelPrefix is a prefix for node channel.
	tarantoolNodeChannelPrefix = internalChannelPrefix + "node."
)

// Broker uses Tarantool to implement centrifuge.Broker functionality.
type Broker struct {
	controlRound uint64 // Keep atomic on struct top for 32-bit architectures.
	node         *centrifuge.Node
	sharding     bool
	config       BrokerConfig
	shards       []*Shard
	nodeChannel  string
}

var _ centrifuge.Broker = (*Broker)(nil)

// BrokerConfig is a config for Tarantool Broker.
type BrokerConfig struct {
	// HistoryMetaTTL sets a time of stream meta key expiration in Tarantool. Stream
	// meta key is a Tarantool HASH that contains top offset in channel and epoch value.
	// By default stream meta keys do not expire.
	HistoryMetaTTL time.Duration

	// UsePolling allows to turn on polling mode instead of push.
	UsePolling bool

	// Shards is a list of Tarantool instances to shard data by channel.
	Shards []*Shard
}

// NewBroker initializes Tarantool Broker.
func NewBroker(n *centrifuge.Node, config BrokerConfig) (*Broker, error) {
	if len(config.Shards) == 0 {
		return nil, errors.New("no Tarantool shards provided in configuration")
	}
	if len(config.Shards) > 1 {
		n.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, fmt.Sprintf("Tarantool sharding enabled: %d shards", len(config.Shards))))
	}
	e := &Broker{
		node:        n,
		shards:      config.Shards,
		config:      config,
		sharding:    len(config.Shards) > 1,
		nodeChannel: nodeChannel(n.ID()),
	}
	return e, nil
}

// Run runs broker after node initialized.
func (b *Broker) Run(h centrifuge.BrokerEventHandler) error {
	for _, shard := range b.shards {
		if err := b.runShard(shard, h); err != nil {
			return err
		}
	}
	return nil
}

func (b *Broker) runForever(fn func(), minDelay time.Duration) {
	for {
		started := time.Now()
		fn()
		elapsed := time.Since(started)
		if elapsed < minDelay {
			// Sleep for a while to prevent busy loop when reconnecting.
			// If elapsed >= minDelay then fn will be restarted right away – this is
			// intentional for fast reconnect in case of one random error.
			time.Sleep(minDelay - elapsed)
		}
	}
}

const pubSubRoutineMinDelay = 300 * time.Millisecond

// Run Tarantool shard.
func (b *Broker) runShard(s *Shard, h centrifuge.BrokerEventHandler) error {
	b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, fmt.Sprintf("start Tarantool shard: %v", tools.GetLogAddresses(s.config.Addresses))))
	go b.runForever(func() {
		b.runPubSub(s, h)
	}, pubSubRoutineMinDelay)
	go b.runForever(func() {
		b.runControlPubSub(s, h)
	}, pubSubRoutineMinDelay)
	return nil
}

type pubRequest struct {
	MsgType        string
	Channel        string
	Data           string
	HistoryTTL     int
	HistorySize    int
	HistoryMetaTTL int
}

type pubResponse struct {
	Offset uint64
	Epoch  string
}

func (m *pubResponse) DecodeMsgpack(d *msgpack.Decoder) error {
	var err error
	var l int
	if l, err = d.DecodeArrayLen(); err != nil {
		return err
	}
	if l != 2 {
		return fmt.Errorf("malformed array len: %d", l)
	}
	if m.Offset, err = d.DecodeUint64(); err != nil {
		return err
	}
	if m.Epoch, err = d.DecodeString(); err != nil {
		return err
	}
	return nil
}

// Publish - see centrifuge.Broker interface description.
func (b *Broker) Publish(ch string, data []byte, opts centrifuge.PublishOptions) (centrifuge.StreamPosition, error) {
	s := consistentShard(ch, b.shards)

	protoPub := &protocol.Publication{
		Data: data,
		Info: infoToProto(opts.ClientInfo),
		Tags: opts.Tags,
	}
	byteMessage, err := protoPub.MarshalVT()
	if err != nil {
		return centrifuge.StreamPosition{}, err
	}
	pr := &pubRequest{
		MsgType:        "p",
		Channel:        ch,
		Data:           string(byteMessage),
		HistoryTTL:     int(opts.HistoryTTL.Seconds()),
		HistorySize:    opts.HistorySize,
		HistoryMetaTTL: int(b.config.HistoryMetaTTL.Seconds()),
	}
	var resp pubResponse
	err = s.ExecTyped(tarantool.Call("centrifuge.publish", pr), &resp)
	if err != nil {
		return centrifuge.StreamPosition{}, err
	}
	return centrifuge.StreamPosition{Offset: resp.Offset, Epoch: resp.Epoch}, err
}

// PublishJoin - see centrifuge.Broker interface description.
func (b *Broker) PublishJoin(ch string, info *centrifuge.ClientInfo) error {
	s := consistentShard(ch, b.shards)
	pr := pubRequest{
		MsgType: "j",
		Channel: ch,
		Data:    b.clientInfoString(info),
	}
	_, err := s.Exec(tarantool.Call("centrifuge.publish", pr))
	return err
}

// PublishLeave - see centrifuge.Broker interface description.
func (b *Broker) PublishLeave(ch string, info *centrifuge.ClientInfo) error {
	s := consistentShard(ch, b.shards)
	pr := pubRequest{
		MsgType: "l",
		Channel: ch,
		Data:    b.clientInfoString(info),
	}
	_, err := s.Exec(tarantool.Call("centrifuge.publish", pr))
	return err
}

func (b *Broker) clientInfoString(clientInfo *centrifuge.ClientInfo) string {
	var info string
	if clientInfo != nil {
		byteMessage, err := infoToProto(clientInfo).MarshalVT()
		if err != nil {
			return info
		}
		info = string(byteMessage)
	}
	return info
}

// PublishControl - see centrifuge.Broker interface description.
func (b *Broker) PublishControl(data []byte, nodeID, _ string) error {
	currentRound := atomic.AddUint64(&b.controlRound, 1)
	index := currentRound % uint64(len(b.shards))
	var channel string
	if nodeID != "" {
		channel = nodeChannel(nodeID)
	} else {
		channel = b.controlChannel()
	}
	pr := pubRequest{
		MsgType: "c",
		Channel: channel,
		Data:    string(data),
	}
	_, err := b.shards[index].Exec(tarantool.Call("centrifuge.publish", pr))
	return err
}

func (b *Broker) controlChannel() string {
	return tarantoolControlChannel
}

func nodeChannel(nodeID string) string {
	return tarantoolNodeChannelPrefix + nodeID
}

// Subscribe - see centrifuge.Broker interface description.
func (b *Broker) Subscribe(ch string) error {
	if strings.HasPrefix(ch, internalChannelPrefix) {
		return centrifuge.ErrorBadRequest
	}
	if b.node.LogEnabled(centrifuge.LogLevelDebug) {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "subscribe node on channel", map[string]interface{}{"channel": ch}))
	}
	r := newSubRequest([]string{ch}, true)
	s := b.shards[consistentIndex(ch, len(b.shards))]
	return b.sendSubscribe(s, r)
}

// Unsubscribe - see centrifuge.Broker interface description.
func (b *Broker) Unsubscribe(ch string) error {
	if b.node.LogEnabled(centrifuge.LogLevelDebug) {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "unsubscribe node from channel", map[string]interface{}{"channel": ch}))
	}
	r := newSubRequest([]string{ch}, false)
	s := b.shards[consistentIndex(ch, len(b.shards))]
	return b.sendSubscribe(s, r)
}

var errOpTimeout = errors.New("operation timed out")

func (b *Broker) sendSubscribe(shard *Shard, r subRequest) error {
	select {
	case shard.subCh <- r:
	default:
		timer := AcquireTimer(defaultRequestTimeout)
		defer ReleaseTimer(timer)
		select {
		case shard.subCh <- r:
		case <-timer.C:
			return errOpTimeout
		}
	}
	return r.result()
}

type historyRequest struct {
	Channel        string
	Offset         uint64
	Limit          int
	Reverse        bool
	IncludePubs    bool
	HistoryMetaTTL int
}

type historyResponse struct {
	Offset uint64
	Epoch  string
	Pubs   []*centrifuge.Publication
}

func (m *historyResponse) DecodeMsgpack(d *msgpack.Decoder) error {
	var err error
	var l int
	if l, err = d.DecodeArrayLen(); err != nil {
		return err
	}
	if l != 3 {
		return fmt.Errorf("malformed array len: %d", l)
	}
	if m.Offset, err = d.DecodeUint64(); err != nil {
		return err
	}
	if m.Epoch, err = d.DecodeString(); err != nil {
		return err
	}
	if l, err = d.DecodeArrayLen(); err != nil {
		return err
	}
	if l == -1 {
		return nil
	}

	pubs := make([]*centrifuge.Publication, 0, l)

	for i := 0; i < l; i++ {
		var l int
		if l, err = d.DecodeArrayLen(); err != nil {
			return err
		}
		if l != 5 {
			return fmt.Errorf("malformed array len: %d", l)
		}
		if _, err = d.DecodeUint64(); err != nil {
			return err
		}
		if _, err = d.DecodeString(); err != nil {
			return err
		}
		offset, err := d.DecodeUint64()
		if err != nil {
			return err
		}
		if _, err = d.DecodeFloat64(); err != nil {
			return err
		}
		data, err := d.DecodeString()
		if err != nil {
			return err
		}
		var p protocol.Publication
		if err = p.UnmarshalVT([]byte(data)); err != nil {
			return err
		}
		pub := pubFromProto(&p)
		pub.Offset = offset
		pubs = append(pubs, pub)
	}
	m.Pubs = pubs
	return nil
}

func pubFromProto(pub *protocol.Publication) *centrifuge.Publication {
	if pub == nil {
		return nil
	}
	return &centrifuge.Publication{
		Offset: pub.GetOffset(),
		Data:   pub.Data,
		Info:   infoFromProto(pub.GetInfo()),
		Tags:   pub.GetTags(),
	}
}

// History - see centrifuge.Broker interface description.
func (b *Broker) History(ch string, filter centrifuge.HistoryFilter) ([]*centrifuge.Publication, centrifuge.StreamPosition, error) {
	var includePubs = true
	var offset uint64
	if filter.Since != nil {
		if filter.Reverse {
			offset = filter.Since.Offset - 1
			if offset == 0 {
				includePubs = false
			}
		} else {
			offset = filter.Since.Offset + 1
		}
	}
	var limit int
	if filter.Limit == 0 {
		includePubs = false
	}
	if filter.Limit > 0 {
		limit = filter.Limit
	}
	historyMetaTTLSeconds := int(b.config.HistoryMetaTTL.Seconds())
	s := consistentShard(ch, b.shards)
	req := historyRequest{
		Channel:        ch,
		Offset:         offset,
		Limit:          limit,
		Reverse:        filter.Reverse,
		IncludePubs:    includePubs,
		HistoryMetaTTL: historyMetaTTLSeconds,
	}
	var resp historyResponse
	err := s.ExecTyped(tarantool.Call("centrifuge.history", req), &resp)
	if err != nil {
		return nil, centrifuge.StreamPosition{}, err
	}
	streamPosition := centrifuge.StreamPosition{Offset: resp.Offset, Epoch: resp.Epoch}
	return resp.Pubs, streamPosition, nil
}

type removeHistoryRequest struct {
	Channel string
}

// RemoveHistory - see centrifuge.Broker interface description.
func (b *Broker) RemoveHistory(ch string) error {
	s := consistentShard(ch, b.shards)
	_, err := s.Exec(tarantool.Call("centrifuge.remove_history", removeHistoryRequest{Channel: ch}))
	return err
}

const (
	// tarantoolPubSubWorkerChannelSize sets buffer size of channel to which we send all
	// messages received from Tarantool PUB/SUB connection to process in separate goroutine.
	tarantoolPubSubWorkerChannelSize = 512
	// tarantoolSubscribeBatchLimit is a maximum number of channels to include in a single
	// batch subscribe call.
	tarantoolSubscribeBatchLimit = 512
)

func (b *Broker) getShard(channel string) *Shard {
	if !b.sharding {
		return b.shards[0]
	}
	return b.shards[consistentIndex(channel, len(b.shards))]
}

type pollRequest struct {
	ConnID     string
	UsePolling bool
	Timeout    int
}

type subscribeRequest struct {
	ConnID   string
	Channels []string
}

type pubSubMessage struct {
	Type    string
	Channel string
	Offset  uint64
	Epoch   string
	Data    []byte
}

func (m *pubSubMessage) DecodeMsgpack(d *msgpack.Decoder) error {
	var err error
	var l int
	if l, err = d.DecodeArrayLen(); err != nil {
		return err
	}
	if l != 5 {
		return fmt.Errorf("wrong array len: %d", l)
	}
	if m.Type, err = d.DecodeString(); err != nil {
		return err
	}
	if m.Channel, err = d.DecodeString(); err != nil {
		return err
	}
	if m.Offset, err = d.DecodeUint64(); err != nil {
		return err
	}
	if m.Epoch, err = d.DecodeString(); err != nil {
		return err
	}
	if data, err := d.DecodeString(); err != nil {
		return err
	} else {
		m.Data = []byte(data)
	}
	return nil
}

func (b *Broker) runPubSub(s *Shard, eventHandler centrifuge.BrokerEventHandler) {
	logError := func(errString string) {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "restart pub/sub", map[string]interface{}{"error": errString}))
	}

	u, err := uuid.NewRandom()
	if err != nil {
		logError(err.Error())
		return
	}
	connID := u.String()

	conn, cancel, err := s.pubSubConn()
	if err != nil {
		logError(err.Error())
		return
	}
	defer cancel()
	defer func() { _ = conn.Close() }()

	// Register poller with unique ID.
	_, err = conn.Exec(tarantool.Call("centrifuge.get_messages", pollRequest{ConnID: connID, UsePolling: b.config.UsePolling, Timeout: 0}))
	if err != nil {
		logError(err.Error())
		return
	}

	numWorkers := runtime.NumCPU()

	b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, fmt.Sprintf("running Tarantool PUB/SUB, num workers: %d", numWorkers)))
	defer func() {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "stopping Tarantool PUB/SUB"))
	}()

	done := make(chan struct{})
	var doneOnce sync.Once
	closeDoneOnce := func() {
		doneOnce.Do(func() {
			close(done)
			_ = conn.Close()
		})
	}
	defer closeDoneOnce()

	// Run subscriber goroutine.
	go func(conn *tarantool.Connection) {
		for {
			select {
			case <-done:
				return
			case r := <-s.subCh:
				isSubscribe := r.subscribe
				channelBatch := []subRequest{r}

				chIDs := r.channels

				var otherR *subRequest

			loop:
				for len(chIDs) < tarantoolSubscribeBatchLimit {
					select {
					case r := <-s.subCh:
						if r.subscribe != isSubscribe {
							// We can not mix subscribe and unsubscribe request into one batch
							// so must stop here. As we consumed a subRequest value from channel
							// we should take care of it later.
							otherR = &r
							break loop
						}
						channelBatch = append(channelBatch, r)
						chIDs = append(chIDs, r.channels...)
					default:
						break loop
					}
				}

				var opErr error
				if isSubscribe {
					_, err = conn.Exec(tarantool.Call("centrifuge.subscribe", subscribeRequest{ConnID: connID, Channels: chIDs}))
					opErr = err
				} else {
					_, err = conn.Exec(tarantool.Call("centrifuge.unsubscribe", subscribeRequest{ConnID: connID, Channels: chIDs}))
					opErr = err
				}

				if opErr != nil {
					for _, r := range channelBatch {
						r.done(opErr)
					}
					if otherR != nil {
						otherR.done(opErr)
					}
					// Close conn, this should cause Receive to return with err below
					// and whole runPubSub method to restart.
					closeDoneOnce()
					return
				}
				for _, r := range channelBatch {
					r.done(nil)
				}
				if otherR != nil {
					chIDs := otherR.channels
					var opErr error
					if otherR.subscribe {
						_, err = conn.Exec(tarantool.Call("centrifuge.subscribe", subscribeRequest{ConnID: connID, Channels: chIDs}))
						opErr = err
					} else {
						_, err = conn.Exec(tarantool.Call("centrifuge.unsubscribe", subscribeRequest{ConnID: connID, Channels: chIDs}))
						opErr = err
					}
					if opErr != nil {
						otherR.done(opErr)
						// Close conn, this should cause Receive to return with err below
						// and whole runPubSub method to restart.
						closeDoneOnce()
						return
					}
					otherR.done(nil)
				}
			}
		}
	}(conn)

	// Run workers to spread received message processing work over worker goroutines.
	workers := make(map[int]chan pubSubMessage)
	for i := 0; i < numWorkers; i++ {
		workerCh := make(chan pubSubMessage, tarantoolPubSubWorkerChannelSize)
		workers[i] = workerCh
		go func(ch chan pubSubMessage) {
			for {
				select {
				case <-done:
					return
				case n := <-ch:
					err := b.handleMessage(eventHandler, n)
					if err != nil {
						b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error handling client message", map[string]interface{}{"error": err.Error()}))
						continue
					}
				}
			}
		}(workerCh)
	}

	go func() {
		var chIDs []string

		channels := b.node.Hub().Channels()
		for i := 0; i < len(channels); i++ {
			if b.getShard(channels[i]) == s {
				chIDs = append(chIDs, channels[i])
			}
		}

		batch := make([]string, 0)

		for i, ch := range chIDs {
			if len(batch) > 0 && i%tarantoolSubscribeBatchLimit == 0 {
				r := newSubRequest(batch, true)
				err := b.sendSubscribe(s, r)
				if err != nil {
					b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error subscribing", map[string]interface{}{"error": err.Error()}))
					closeDoneOnce()
					return
				}
				batch = nil
			}
			batch = append(batch, ch)
		}
		if len(batch) > 0 {
			r := newSubRequest(batch, true)
			err := b.sendSubscribe(s, r)
			if err != nil {
				b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error subscribing", map[string]interface{}{"error": err.Error()}))
				closeDoneOnce()
				return
			}
		}
	}()

	processPubSubMessages := func(messages []pubSubMessage) {
		for _, msg := range messages {
			// Add message to worker channel preserving message order - i.e. messages
			// from the same channel will be processed in the same worker.
			workers[index(msg.Channel, numWorkers)] <- msg
		}
	}

	for {
		err := b.waitPubSubMessages(conn, connID, processPubSubMessages)
		if err != nil {
			logError(err.Error())
			return
		}
	}
}

func (b *Broker) waitPubSubMessages(conn *tarantool.Connection, connID string, cb func([]pubSubMessage)) error {
	if !b.config.UsePolling {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_, err := conn.ExecContext(ctx, tarantool.Call(
			"centrifuge.get_messages",
			pollRequest{ConnID: connID, UsePolling: b.config.UsePolling, Timeout: 25},
		).WithPushTyped(func(decode func(interface{}) error) {
			var m [][]pubSubMessage
			if err := decode(&m); err != nil {
				b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding push", map[string]interface{}{"error": err.Error()}))
				return
			}
			if len(m) == 1 {
				cb(m[0])
			}
		}))
		if err != nil {
			return err
		}
	} else {
		var m [][]pubSubMessage
		err := conn.ExecTyped(tarantool.Call(
			"centrifuge.get_messages",
			pollRequest{ConnID: connID, UsePolling: b.config.UsePolling, Timeout: 25}),
			&m,
		)
		if err != nil {
			return err
		}
		if len(m) == 1 {
			cb(m[0])
		}
	}
	return nil
}

func (b *Broker) handleMessage(eventHandler centrifuge.BrokerEventHandler, msg pubSubMessage) error {
	switch msg.Type {
	case "p":
		var pub protocol.Publication
		err := pub.UnmarshalVT(msg.Data)
		if err == nil {
			publication := pubFromProto(&pub)
			publication.Offset = msg.Offset
			_ = eventHandler.HandlePublication(msg.Channel, publication, centrifuge.StreamPosition{Offset: msg.Offset, Epoch: msg.Epoch})
		}
	case "j":
		var info protocol.ClientInfo
		err := info.UnmarshalVT(msg.Data)
		if err == nil {
			_ = eventHandler.HandleJoin(msg.Channel, infoFromProto(&info))
		}
	case "l":
		var info protocol.ClientInfo
		err := info.UnmarshalVT(msg.Data)
		if err == nil {
			_ = eventHandler.HandleLeave(msg.Channel, infoFromProto(&info))
		}
	}
	return nil
}

func (b *Broker) runControlPubSub(s *Shard, eventHandler centrifuge.BrokerEventHandler) {
	logError := func(errString string) {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "restart control pub/sub", map[string]interface{}{"error": errString}))
	}

	u, err := uuid.NewRandom()
	if err != nil {
		logError(err.Error())
		return
	}
	connID := u.String()

	conn, cancel, err := s.pubSubConn()
	if err != nil {
		logError(err.Error())
		return
	}
	defer cancel()
	defer func() { _ = conn.Close() }()

	// Register poller with unique ID.
	_, err = conn.Exec(tarantool.Call("centrifuge.get_messages", pollRequest{ConnID: connID, UsePolling: b.config.UsePolling, Timeout: 0}))
	if err != nil {
		logError(err.Error())
		return
	}

	numWorkers := runtime.NumCPU()

	b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, fmt.Sprintf("running Tarantool control PUB/SUB, num workers: %d", numWorkers)))
	defer func() {
		b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "stopping Tarantool control PUB/SUB"))
	}()

	done := make(chan struct{})
	var doneOnce sync.Once
	closeDoneOnce := func() {
		doneOnce.Do(func() {
			close(done)
			_ = conn.Close()
		})
	}
	defer closeDoneOnce()

	// Run workers to spread message processing work over worker goroutines.
	workCh := make(chan pubSubMessage)
	for i := 0; i < numWorkers; i++ {
		go func() {
			for {
				select {
				case <-done:
					return
				case n := <-workCh:
					err := eventHandler.HandleControl(n.Data)
					if err != nil {
						b.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error handling control message", map[string]interface{}{"error": err.Error()}))
						continue
					}
				}
			}
		}()
	}

	controlChannel := b.controlChannel()
	_, err = conn.Exec(tarantool.Call("centrifuge.subscribe", subscribeRequest{ConnID: connID, Channels: []string{controlChannel, b.nodeChannel}}))
	if err != nil {
		logError(err.Error())
		return
	}

	processPubSubMessages := func(messages []pubSubMessage) {
		for _, msg := range messages {
			workCh <- msg
		}
	}

	for {
		err := b.waitPubSubMessages(conn, connID, processPubSubMessages)
		if err != nil {
			logError(err.Error())
			return
		}
	}
}
