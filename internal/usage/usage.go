package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v4/internal/build"
	"github.com/centrifugal/centrifugo/v4/internal/rule"

	"github.com/centrifugal/centrifuge"
)

var statsRand *rand.Rand

var initialDelay time.Duration
var tickInterval time.Duration
var sendInterval time.Duration
var metricsPrefix string

func init() {
	statsRand = rand.New(rand.NewSource(time.Now().Unix()))

	// Initial delay in between 24-48h. Using minute resolution here
	// is intentional to get a better time spread.
	initialDelay = time.Duration(statsRand.Intn(24*60)+24*60) * time.Minute
	tickInterval = time.Hour
	sendInterval = 24 * time.Hour
	metricsPrefix = "centrifugo."

	// Uncomment during development (for faster timings and test prefix).
	//initialDelay = time.Duration(statsRand.Intn(30)+1) * time.Second
	//tickInterval = 10 * time.Second
	//sendInterval = time.Minute
	//metricsPrefix = "test."
}

// Sender can send anonymous usage stats. Centrifugo does not collect any sensitive info.
// Only impersonal counters to estimate installation size distribution and feature use.
type Sender struct {
	mu             sync.RWMutex
	node           *centrifuge.Node
	rules          *rule.Container
	features       Features
	maxNumNodes    int
	maxNumClients  int
	maxNumChannels int
	lastSentAt     int64
}

// Features is a helper struct to build metrics.
type Features struct {
	// Build info.
	Version string
	Edition string

	// Engine or broker usage.
	Engine     string
	EngineMode string
	Broker     string
	BrokerMode string

	// Transports.
	Websocket     bool
	HTTPStream    bool
	SSE           bool
	SockJS        bool
	UniWebsocket  bool
	UniGRPC       bool
	UniSSE        bool
	UniHTTPStream bool

	// Proxies.
	ConnectProxy   bool
	RefreshProxy   bool
	SubscribeProxy bool
	PublishProxy   bool
	RPCProxy       bool

	// Uses GRPC server API.
	GrpcAPI bool
	// Admin interface enabled.
	Admin bool
	// Uses automatic personal channel subscribe.
	SubscribeToPersonal bool

	// PRO features.
	ClickhouseAnalytics bool
	UserStatus          bool
	Throttling          bool
	Singleflight        bool
}

// NewSender creates usage stats sender.
func NewSender(node *centrifuge.Node, rules *rule.Container, features Features) *Sender {
	return &Sender{
		node:     node,
		rules:    rules,
		features: features,
	}
}

const (
	// LastSentUpdateNotificationOp is an op for Centrifuge Notification in which we
	// send last sent time to all nodes.
	LastSentUpdateNotificationOp = "usage_stats.last_sent_at"
)

func (s *Sender) isDev() bool {
	return s.features.Version == "0.0.0"
}

// Start sending usage stats. How it works:
// First send in between 24-48h from node start.
// After the initial delay has passed: every hour check last time stats were sent by all
// the nodes in a Centrifugo cluster. If no points were sent in last 24h, then push metrics
// and update push time on all nodes (broadcast current time). There is still a chance of
// duplicate data sending – but should be rare and tolerable for the purpose.
func (s *Sender) Start(ctx context.Context) {
	firstTimeSend := time.Now().Add(initialDelay)
	if s.isDev() {
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: schedule next send", map[string]interface{}{"delay": initialDelay.String()}))
	}

	// Wait 1/4 of a delay to randomize hourly ticks on different nodes.
	select {
	case <-ctx.Done():
		return
	case <-time.After(initialDelay / 4):
	}

	if s.isDev() {
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: start periodic ticks", map[string]interface{}{}))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(tickInterval):
			if s.isDev() {
				s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: updating max values", map[string]interface{}{}))
			}
			err := s.updateMaxValues()
			if err != nil {
				if s.isDev() {
					s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "usage stats: error updating max values", map[string]interface{}{"error": err.Error()}))
				}
				continue
			}

			if time.Now().Before(firstTimeSend) {
				if s.isDev() {
					s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: too early to send first time", map[string]interface{}{}))
				}
				continue
			}

			s.mu.RLock()
			lastSentAt := s.lastSentAt
			s.mu.RUnlock()
			if lastSentAt > 0 {
				s.broadcastLastSentAt()
			}

			if lastSentAt > 0 && time.Now().Unix() <= lastSentAt+int64(sendInterval.Seconds()) {
				if s.isDev() {
					s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: too early to send", map[string]interface{}{}))
				}
				continue
			}

			if s.isDev() {
				s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: sending usage stats", map[string]interface{}{}))
			}
			err = s.sendUsageStats(s.prepareMetrics(), build.UsageStatsEndpoint, build.UsageStatsToken)
			if err != nil {
				if s.isDev() {
					s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "usage stats: error sending", map[string]interface{}{"error": err.Error()}))
				}
				continue
			}
			s.mu.Lock()
			s.lastSentAt = time.Now().Unix()
			s.resetMaxValues()
			s.mu.Unlock()
			s.broadcastLastSentAt()
		}
	}
}

type lastSentAtEnvelope struct {
	LastSentAt int64 `json:"lastSentAt"`
}

func (s *Sender) broadcastLastSentAt() {
	s.mu.RLock()
	envelope := lastSentAtEnvelope{
		LastSentAt: s.lastSentAt,
	}
	data, _ := json.Marshal(envelope)
	s.mu.RUnlock()
	if err := s.node.Notify(LastSentUpdateNotificationOp, data, ""); err != nil {
		// Issue a single retry.
		if err = s.node.Notify(LastSentUpdateNotificationOp, data, ""); err != nil {
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "usage stats: error broadcasting stats lastSentAt", map[string]interface{}{"error": err.Error()}))
		}
	}
}

// UpdateLastSentAt sets the lastSentAt received from other node only
// if received value greater than local one (so that we can avoid sending
// duplicated stats).
func (s *Sender) UpdateLastSentAt(data []byte) {
	var envelope lastSentAtEnvelope
	err := json.Unmarshal(data, &envelope)
	if err != nil {
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "usage stats: error decoding lastSentAtEnvelope", map[string]interface{}{"error": err.Error()}))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if envelope.LastSentAt > s.lastSentAt {
		if s.isDev() {
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: updating last sent to value from another node", map[string]interface{}{}))
		}
		s.lastSentAt = envelope.LastSentAt
		s.resetMaxValues()
	}
}

func (s *Sender) updateMaxValues() error {
	info, err := s.node.Info()
	if err != nil {
		return fmt.Errorf("usage stats: error getting info: %w", err)
	}

	numNodes := len(info.Nodes)

	numClients := 0
	for _, node := range info.Nodes {
		numClients += int(node.NumClients)
	}

	numChannels := 0
	for _, node := range info.Nodes {
		numChannels += int(node.NumChannels)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if numNodes > s.maxNumNodes {
		s.maxNumNodes = numNodes
	}

	if numClients > s.maxNumClients {
		s.maxNumClients = numClients
	}

	if numChannels > s.maxNumChannels {
		s.maxNumChannels = numChannels
	}

	return nil
}

func getHistogramMetric(val int, bounds []int, metricPrefix string) string {
	for _, bound := range bounds {
		if val <= bound {
			boundStr := strconv.Itoa(bound)
			if strings.HasSuffix(boundStr, "000000") {
				boundStr = strings.TrimSuffix(boundStr, "000000")
				boundStr += "m"
			} else if strings.HasSuffix(boundStr, "000") {
				boundStr = strings.TrimSuffix(boundStr, "000")
				boundStr += "k"
			}
			return metricPrefix + "le_" + boundStr
		}
	}
	return metricPrefix + "le_inf"
}

// Lock must be held outside.
func (s *Sender) resetMaxValues() {
	s.maxNumNodes = 0
	s.maxNumClients = 0
	s.maxNumChannels = 0
}

func (s *Sender) prepareMetrics() []*metric {
	now := time.Now().Unix()

	createPoint := func(name string) *metric {
		finalName := metricsPrefix + "stats." + name
		md := metric{
			Name:     finalName,
			Metric:   finalName,
			Interval: int(sendInterval.Seconds()),
			Value:    1,
			Time:     now,
			Type:     "count",
		}
		md.SetId()
		return &md
	}

	version := strings.Replace(s.features.Version, ".", "_", -1)
	edition := strings.ToLower(s.features.Edition)
	engineMode := s.features.EngineMode
	if engineMode == "" {
		engineMode = "default"
	}
	brokerMode := s.features.BrokerMode
	if brokerMode == "" {
		brokerMode = "default"
	}

	var metrics []*metric

	metrics = append(metrics, createPoint("total"))
	metrics = append(metrics, createPoint("version."+version+".edition."+edition))
	metrics = append(metrics, createPoint("arch."+runtime.GOARCH+".os."+runtime.GOOS))

	if s.features.Broker == "" {
		metrics = append(metrics, createPoint("engine."+s.features.Engine+".mode."+engineMode))
	} else {
		metrics = append(metrics, createPoint("broker."+s.features.Broker+".mode."+brokerMode))
	}

	if s.features.Websocket {
		metrics = append(metrics, createPoint("transports_enabled.websocket"))
	}
	if s.features.HTTPStream {
		metrics = append(metrics, createPoint("transports_enabled.http_stream"))
	}
	if s.features.SSE {
		metrics = append(metrics, createPoint("transports_enabled.sse"))
	}
	if s.features.SockJS {
		metrics = append(metrics, createPoint("transports_enabled.sockjs"))
	}
	if s.features.UniWebsocket {
		metrics = append(metrics, createPoint("transports_enabled.uni_websocket"))
	}
	if s.features.UniHTTPStream {
		metrics = append(metrics, createPoint("transports_enabled.uni_http_stream"))
	}
	if s.features.UniSSE {
		metrics = append(metrics, createPoint("transports_enabled.uni_sse"))
	}
	if s.features.UniGRPC {
		metrics = append(metrics, createPoint("transports_enabled.uni_grpc"))
	}
	if s.features.ConnectProxy {
		metrics = append(metrics, createPoint("proxies_enabled.connect"))
	}
	if s.features.RefreshProxy {
		metrics = append(metrics, createPoint("proxies_enabled.refresh"))
	}
	if s.features.SubscribeProxy {
		metrics = append(metrics, createPoint("proxies_enabled.subscribe"))
	}
	if s.features.PublishProxy {
		metrics = append(metrics, createPoint("proxies_enabled.publish"))
	}
	if s.features.RPCProxy {
		metrics = append(metrics, createPoint("proxies_enabled.rpc"))
	}
	if s.features.GrpcAPI {
		metrics = append(metrics, createPoint("features_enabled.grpc_api"))
	}
	if s.features.SubscribeToPersonal {
		metrics = append(metrics, createPoint("features_enabled.user_subscribe_to_personal"))
	}
	if s.features.Admin {
		metrics = append(metrics, createPoint("features_enabled.admin_ui"))
	}
	if s.features.ClickhouseAnalytics {
		metrics = append(metrics, createPoint("features_enabled.clickhouse_analytics"))
	}
	if s.features.UserStatus {
		metrics = append(metrics, createPoint("features_enabled.user_status"))
	}
	if s.features.Throttling {
		metrics = append(metrics, createPoint("features_enabled.throttling"))
	}
	if s.features.Singleflight {
		metrics = append(metrics, createPoint("features_enabled.singleflight"))
	}

	var usesHistory bool
	var usesPresence bool
	var usesJoinLeave bool

	namespaces := s.rules.Config().Namespaces
	chOpts := s.rules.Config().ChannelOptions
	if chOpts.HistoryTTL > 0 && chOpts.HistorySize > 0 {
		usesHistory = true
	}
	if chOpts.Presence {
		usesPresence = true
	}
	if chOpts.JoinLeave {
		usesJoinLeave = true
	}
	for _, ns := range namespaces {
		chOpts = ns.ChannelOptions
		if chOpts.HistoryTTL > 0 && chOpts.HistorySize > 0 {
			usesHistory = true
		}
		if chOpts.Presence {
			usesPresence = true
		}
		if chOpts.JoinLeave {
			usesJoinLeave = true
		}
	}

	if usesHistory {
		metrics = append(metrics, createPoint("features_enabled.history"))
	}
	if usesPresence {
		metrics = append(metrics, createPoint("features_enabled.presence"))
	}
	if usesJoinLeave {
		metrics = append(metrics, createPoint("features_enabled.join_leave"))
	}

	s.mu.RLock()
	numNodesMetric := getHistogramMetric(
		s.maxNumNodes,
		[]int{1, 2, 3, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000},
		"num_nodes.",
	)

	numClientsMetric := getHistogramMetric(
		s.maxNumClients,
		[]int{
			0, 5, 10, 100, 1000, 10000, 50000, 100000,
			500000, 1000000, 5000000, 10000000, 50000000,
			100000000,
		},
		"num_clients.",
	)

	numChannelsMetric := getHistogramMetric(
		s.maxNumChannels,
		[]int{
			0, 5, 10, 100, 1000, 10000, 50000, 100000,
			500000, 1000000, 5000000, 10000000, 50000000,
			100000000,
		},
		"num_channels.",
	)

	clientsPerNode := s.maxNumClients / s.maxNumNodes
	if clientsPerNode >= 1000 {
		// Insights about how many client connections per node users have.
		// We are not interested in too low numbers here.
		numClientsPerNodeMetric := getHistogramMetric(
			clientsPerNode,
			[]int{
				1000, 5000, 10000, 20000, 50000,
				100000, 200000, 500000, 1000000, 5000000,
				10000000,
			},
			"num_clients_per_node.",
		)
		metrics = append(metrics, createPoint(numClientsPerNodeMetric))
	}
	s.mu.RUnlock()

	metrics = append(metrics, createPoint(numNodesMetric))
	metrics = append(metrics, createPoint(numClientsMetric))
	metrics = append(metrics, createPoint(numChannelsMetric))
	metrics = append(metrics, createPoint("by_edition."+edition+"."+numNodesMetric))
	metrics = append(metrics, createPoint("by_edition."+edition+"."+numClientsMetric))
	metrics = append(metrics, createPoint("by_edition."+edition+"."+numChannelsMetric))

	numNamespaces := s.rules.NumNamespaces()
	numNamespacesMetric := getHistogramMetric(
		numNamespaces,
		[]int{0, 1, 2, 5, 10, 50, 100, 500, 1000},
		"num_namespaces.",
	)
	metrics = append(metrics, createPoint(numNamespacesMetric))

	numRpcNamespaces := s.rules.NumRpcNamespaces()
	numRpcNamespacesMetric := getHistogramMetric(
		numRpcNamespaces,
		[]int{0, 1, 2, 5, 10, 50, 100, 500, 1000},
		"num_rpc_namespaces.",
	)
	metrics = append(metrics, createPoint(numRpcNamespacesMetric))
	return metrics
}

func (s *Sender) sendUsageStats(metrics []*metric, statsEndpoint, statsToken string) error {
	data, err := json.Marshal(metrics)
	if err != nil {
		return err
	}

	if s.isDev() {
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: sending usage stats", map[string]interface{}{"payload": string(data)}))
	} else {
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelTrace, "usage stats: sending usage stats", map[string]interface{}{"payload": string(data)}))
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	if statsEndpoint == "" {
		if s.isDev() {
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: skip sending due to empty endpoint", map[string]interface{}{}))
		}
		return nil
	}

	if statsToken == "" {
		if s.isDev() {
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: skip sending due to empty token", map[string]interface{}{}))
		}
		return nil
	}

	endpoints := strings.Split(statsEndpoint, ",")

	var success bool

	for _, endpoint := range endpoints {
		if endpoint == "" {
			continue
		}

		req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(data))
		if err != nil {
			if s.isDev() {
				s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: can't create send request", map[string]interface{}{"error": err.Error()}))
			}
			continue
		}

		req.Header.Add("Authorization", "Bearer "+statsToken)
		req.Header.Add("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			if s.isDev() {
				s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: error sending request", map[string]interface{}{"error": err.Error()}))
			}
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if s.isDev() {
				s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: unexpected response status code", map[string]interface{}{"status": resp.StatusCode}))
			}
			continue
		}
		// If at least one of the endpoints received data it means success for us.
		success = true
	}

	if !success {
		return errors.New("all endpoints failed")
	}

	return nil
}
