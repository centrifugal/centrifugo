package proxy

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v5/internal/rule"
	"github.com/centrifugal/centrifugo/v5/internal/subsource"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

// SubscribeStreamHandlerConfig ...
type SubscribeStreamHandlerConfig struct {
	Proxies           map[string]*SubscribeStreamProxy
	GranularProxyMode bool
}

// SubscribeStreamHandler ...
type SubscribeStreamHandler struct {
	config SubscribeStreamHandlerConfig

	summary           prometheus.Observer
	histogram         prometheus.Observer
	errors            prometheus.Counter
	granularSummary   map[string]prometheus.Observer
	granularHistogram map[string]prometheus.Observer
	granularErrors    map[string]prometheus.Counter
}

// NewSubscribeStreamHandler ...
func NewSubscribeStreamHandler(c SubscribeStreamHandlerConfig) *SubscribeStreamHandler {
	h := &SubscribeStreamHandler{
		config: c,
	}
	if h.config.GranularProxyMode {
		summary := map[string]prometheus.Observer{}
		histogram := map[string]prometheus.Observer{}
		errCounters := map[string]prometheus.Counter{}
		for k := range c.Proxies {
			name := k
			if name == "" {
				name = "__default__"
			}
			summary[name] = granularProxyCallDurationSummary.WithLabelValues("subscribe_stream", name)
			histogram[name] = granularProxyCallDurationHistogram.WithLabelValues("subscribe_stream", name)
			errCounters[name] = granularProxyCallErrorCount.WithLabelValues("subscribe_stream", name)
		}
		h.granularSummary = summary
		h.granularHistogram = histogram
		h.granularErrors = errCounters
	} else {
		h.summary = proxyCallDurationSummary.WithLabelValues("grpc", "subscribe_stream")
		h.histogram = proxyCallDurationHistogram.WithLabelValues("grpc", "subscribe_stream")
		h.errors = proxyCallErrorCount.WithLabelValues("grpc", "subscribe_stream")
	}
	return h
}

// StreamPublishFunc ...
type StreamPublishFunc func(data []byte) error

// SubscribeStreamHandlerFunc ...
type SubscribeStreamHandlerFunc func(
	Client, bool, centrifuge.SubscribeEvent, rule.ChannelOptions, PerCallData,
) (centrifuge.SubscribeReply, StreamPublishFunc, func(), error)

// Handle ...
func (h *SubscribeStreamHandler) Handle(node *centrifuge.Node) SubscribeStreamHandlerFunc {
	return func(
		client Client, bidi bool, e centrifuge.SubscribeEvent,
		chOpts rule.ChannelOptions, pcd PerCallData,
	) (centrifuge.SubscribeReply, StreamPublishFunc, func(), error) {
		started := time.Now()

		var p *SubscribeStreamProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errCounter prometheus.Counter

		if h.config.GranularProxyMode {
			proxyName := chOpts.SubscribeStreamProxyName
			if proxyName == "" {
				node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe stream proxy not configured for a channel", map[string]any{"channel": e.Channel}))
				return centrifuge.SubscribeReply{}, nil, nil, centrifuge.ErrorNotAvailable
			}
			p = h.config.Proxies[proxyName]
			summary = h.granularSummary[proxyName]
			histogram = h.granularHistogram[proxyName]
			errCounter = h.granularErrors[proxyName]
		} else {
			p = h.config.Proxies[""]
			summary = h.summary
			histogram = h.histogram
			errCounter = h.errors
		}

		req := &proxyproto.SubscribeRequest{
			Client:    client.ID(),
			Protocol:  string(client.Transport().Protocol()),
			Transport: client.Transport().Name(),
			Encoding:  getEncoding(p.config.BinaryEncoding),

			User:    client.UserID(),
			Channel: e.Channel,
			Token:   e.Token,
		}
		if !p.config.BinaryEncoding {
			req.Data = e.Data
		} else {
			req.B64Data = base64.StdEncoding.EncodeToString(e.Data)
		}
		if p.config.IncludeConnectionMeta && pcd.Meta != nil {
			req.Meta = proxyproto.Raw(pcd.Meta)
		}

		subscriptionReady := make(chan struct{})
		var subscriptionReadyOnce sync.Once

		subscribeRep, publishFunc, cancelFunc, err := p.SubscribeStream(
			client.Context(),
			bidi,
			req,
			func(pub *proxyproto.Publication, err error) {
				subscriptionReadyOnce.Do(func() {
					select {
					case <-subscriptionReady:
					case <-client.Context().Done():
						return
					}
				})
				select {
				case <-client.Context().Done():
					return
				default:
				}
				if err != nil {
					if errors.Is(err, io.EOF) {
						client.Unsubscribe(e.Channel, centrifuge.Unsubscribe{
							Code:   centrifuge.UnsubscribeCodeServer,
							Reason: "server unsubscribe",
						})
						return
					}
					client.Unsubscribe(e.Channel, centrifuge.Unsubscribe{
						Code:   centrifuge.UnsubscribeCodeInsufficient,
						Reason: "insufficient state",
					})
					return
				}
				_ = client.WritePublication(e.Channel, &centrifuge.Publication{
					Data: pub.Data,
					Tags: pub.Tags,
				}, centrifuge.StreamPosition{})
			},
		)

		duration := time.Since(started).Seconds()

		if err != nil {
			select {
			case <-client.Context().Done():
				// Client connection already closed.
				return centrifuge.SubscribeReply{}, nil, nil, centrifuge.DisconnectConnectionClosed
			default:
			}
			summary.Observe(duration)
			histogram.Observe(duration)
			errCounter.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error from subscribe stream proxy", map[string]any{"error": err.Error()}))
			//proxyCallErrorCount.WithLabelValues(proxyName, "subscribe", "internal").Inc()
			return centrifuge.SubscribeReply{}, nil, nil, err
		}

		summary.Observe(duration)
		histogram.Observe(duration)

		if subscribeRep.Disconnect != nil {
			//proxyCallErrorCount.WithLabelValues(proxyName, "subscribe", "disconnect_"+strconv.FormatUint(uint64(subscribeRep.Disconnect.Code), 10)).Inc()
			return centrifuge.SubscribeReply{}, nil, nil, &centrifuge.Disconnect{
				Code:   subscribeRep.Disconnect.Code,
				Reason: subscribeRep.Disconnect.Reason,
			}
		}
		if subscribeRep.Error != nil {
			//proxyCallErrorCount.WithLabelValues(proxyName, "subscribe", "error_"+strconv.FormatUint(uint64(subscribeRep.Error.Code), 10)).Inc()
			return centrifuge.SubscribeReply{}, nil, nil, &centrifuge.Error{
				Code:    subscribeRep.Error.Code,
				Message: subscribeRep.Error.Message,
			}
		}

		presence := chOpts.Presence
		joinLeave := chOpts.JoinLeave
		pushJoinLeave := chOpts.ForcePushJoinLeave

		var info []byte
		var data []byte
		var expireAt int64

		if subscribeRep.Result != nil {
			if subscribeRep.Result.B64Info != "" {
				decodedInfo, err := base64.StdEncoding.DecodeString(subscribeRep.Result.B64Info)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 info", map[string]any{"client": client.ID(), "error": err.Error()}))
					return centrifuge.SubscribeReply{}, nil, nil, centrifuge.ErrorInternal
				}
				info = decodedInfo
			} else {
				info = subscribeRep.Result.Info
			}
			if subscribeRep.Result.B64Data != "" {
				decodedData, err := base64.StdEncoding.DecodeString(subscribeRep.Result.B64Data)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 data", map[string]any{"client": client.ID(), "error": err.Error()}))
					return centrifuge.SubscribeReply{}, nil, nil, centrifuge.ErrorInternal
				}
				data = decodedData
			} else {
				data = subscribeRep.Result.Data
			}

			result := subscribeRep.Result

			if result.Override != nil && result.Override.Presence != nil {
				presence = result.Override.Presence.Value
			}
			if result.Override != nil && result.Override.JoinLeave != nil {
				joinLeave = result.Override.JoinLeave.Value
			}
			if result.Override != nil && result.Override.ForcePushJoinLeave != nil {
				pushJoinLeave = result.Override.ForcePushJoinLeave.Value
			}

			expireAt = result.ExpireAt
		}

		return centrifuge.SubscribeReply{
			Options: centrifuge.SubscribeOptions{
				ExpireAt:          expireAt,
				ChannelInfo:       info,
				EmitPresence:      presence,
				EmitJoinLeave:     joinLeave,
				PushJoinLeave:     pushJoinLeave,
				EnableRecovery:    false, // Not used for subscribe stream proxy.
				EnablePositioning: false, // Not used for subscribe stream proxy.
				Data:              data,
				Source:            subsource.StreamProxy,
				HistoryMetaTTL:    0, // Not used for subscribe stream proxy.
			},
			ClientSideRefresh: true,
			SubscriptionReady: subscriptionReady,
		}, publishFunc, cancelFunc, nil
	}
}

type OnPublication func(pub *proxyproto.Publication, err error)

type ChannelStreamReader interface {
	Recv() (*proxyproto.StreamSubscribeResponse, error)
}

// SubscribeStream ...
func (p *SubscribeStreamProxy) SubscribeStream(
	ctx context.Context,
	bidi bool,
	sr *proxyproto.SubscribeRequest,
	pubFunc OnPublication,
) (*proxyproto.SubscribeResponse, StreamPublishFunc, func(), error) {
	ctx, cancel := context.WithCancel(ctx)

	var stream ChannelStreamReader

	var publishFunc StreamPublishFunc

	if bidi {
		bidiStream, err := p.SubscribeBidirectional(ctx)
		if err != nil {
			cancel()
			return nil, nil, nil, err
		}
		err = bidiStream.Send(&proxyproto.StreamSubscribeRequest{
			SubscribeRequest: sr,
		})
		if err != nil {
			cancel()
			return nil, nil, nil, err
		}
		stream = bidiStream.(ChannelStreamReader)
		publishFunc = func(data []byte) error {
			return bidiStream.Send(&proxyproto.StreamSubscribeRequest{
				Publication: &proxyproto.Publication{
					Data: data,
				},
			})
		}
	} else {
		var err error
		stream, err = p.SubscribeUnidirectional(ctx, sr)
		if err != nil {
			cancel()
			return nil, nil, nil, err
		}
	}

	firstMessageReceived := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			cancel()
			return
		case <-time.After(time.Duration(p.config.Timeout)):
			cancel()
			return
		case <-firstMessageReceived:
		}
	}()

	resp, err := stream.Recv()
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	close(firstMessageReceived)

	go func() {
		for {
			pubResp, err := stream.Recv()
			if err != nil {
				cancel()
				pubFunc(nil, err)
				return
			}
			pub := pubResp.GetPublication()
			if pub != nil {
				// TODO: better handling of unexpected nil publication.
				pubFunc(pub, nil)
			}
		}
	}()

	if resp.SubscribeResponse == nil {
		return nil, nil, nil, errors.New("no subscribe response in first message from stream proxy")
	}
	return resp.SubscribeResponse, publishFunc, cancel, nil
}
