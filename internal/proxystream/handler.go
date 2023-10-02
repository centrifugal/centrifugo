package proxystream

import (
	"context"
	"errors"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v5/internal/rule"
	"github.com/centrifugal/centrifugo/v5/internal/subsource"

	"github.com/centrifugal/centrifuge"
)

// HandlerConfig ...
type HandlerConfig struct {
	SubscribeProxies map[string]*Proxy
}

// Handler ...
type Handler struct {
	config HandlerConfig
}

// NewHandler ...
func NewHandler(c HandlerConfig) *Handler {
	return &Handler{
		config: c,
	}
}

type PublishFunc func(data []byte) error

// SubscribeHandlerFunc ...
type SubscribeHandlerFunc func(Client, bool, centrifuge.SubscribeEvent, rule.ChannelOptions, PerCallData) (centrifuge.SubscribeReply, PublishFunc, func(), error)

// HandleSubscribe ...
func (h *Handler) HandleSubscribe(node *centrifuge.Node) SubscribeHandlerFunc {
	return func(c Client, bidi bool, e centrifuge.SubscribeEvent, chOpts rule.ChannelOptions, pcd PerCallData) (centrifuge.SubscribeReply, PublishFunc, func(), error) {
		proxyName := "__default__"
		proxyCallTotalCount.WithLabelValues(proxyName, "subscribe").Inc()

		p := h.config.SubscribeProxies[""] // Granular proxy mode not yet available for stream proxies, we only have global one.

		req := &proxyproto.SubscribeRequest{
			Client:    c.ID(),
			Protocol:  string(c.Transport().Protocol()),
			Transport: c.Transport().Name(),

			User:    c.UserID(),
			Channel: e.Channel,
		}
		req.Data = e.Data
		if p.config.IncludeConnectionMeta && pcd.Meta != nil {
			req.Meta = proxyproto.Raw(pcd.Meta)
		}

		subscriptionReady := make(chan struct{})
		var subscriptionReadyOnce sync.Once

		subscribeRep, publishFunc, cancelFunc, err := p.SubscribeStream(c.Context(), bidi, req, func(pub *proxyproto.Publication, err error) {
			subscriptionReadyOnce.Do(func() {
				select {
				case <-subscriptionReady:
				case <-c.Context().Done():
					return
				}
			})
			select {
			case <-c.Context().Done():
				return
			default:
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					c.Unsubscribe(e.Channel, centrifuge.Unsubscribe{
						Code:   centrifuge.UnsubscribeCodeServer,
						Reason: "server unsubscribe",
					})
					return
				}
				c.Unsubscribe(e.Channel, centrifuge.Unsubscribe{
					Code:   centrifuge.UnsubscribeCodeInsufficient,
					Reason: "insufficient state",
				})
				return
			}
			_ = c.WritePublication(e.Channel, &centrifuge.Publication{
				Data: pub.Data,
				Tags: pub.Tags,
			}, centrifuge.StreamPosition{})
		})
		if err != nil {
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error from subscribe stream proxy", map[string]any{"error": err.Error()}))
			proxyCallErrorCount.WithLabelValues(proxyName, "subscribe", "internal").Inc()
			return centrifuge.SubscribeReply{}, nil, nil, err
		}

		if subscribeRep.Disconnect != nil {
			proxyCallErrorCount.WithLabelValues(proxyName, "subscribe", "disconnect_"+strconv.FormatUint(uint64(subscribeRep.Disconnect.Code), 10)).Inc()
			return centrifuge.SubscribeReply{}, nil, nil, &centrifuge.Disconnect{
				Code:   subscribeRep.Disconnect.Code,
				Reason: subscribeRep.Disconnect.Reason,
			}
		}
		if subscribeRep.Error != nil {
			proxyCallErrorCount.WithLabelValues(proxyName, "subscribe", "error_"+strconv.FormatUint(uint64(subscribeRep.Error.Code), 10)).Inc()
			return centrifuge.SubscribeReply{}, nil, nil, &centrifuge.Error{
				Code:    subscribeRep.Error.Code,
				Message: subscribeRep.Error.Message,
			}
		}

		presence := chOpts.Presence
		joinLeave := chOpts.JoinLeave
		pushJoinLeave := chOpts.ForcePushJoinLeave
		recovery := chOpts.ForceRecovery
		positioning := chOpts.ForcePositioning

		var info []byte
		var data []byte
		if subscribeRep.Result != nil {
			result := subscribeRep.Result
			info = result.Info
			data = result.Data
		}

		return centrifuge.SubscribeReply{
			Options: centrifuge.SubscribeOptions{
				ChannelInfo:       info,
				EmitPresence:      presence,
				EmitJoinLeave:     joinLeave,
				PushJoinLeave:     pushJoinLeave,
				EnableRecovery:    recovery,
				EnablePositioning: positioning,
				Data:              data,
				Source:            subsource.StreamProxy,
			},
			ClientSideRefresh: true,
			SubscriptionReady: subscriptionReady,
		}, publishFunc, cancelFunc, nil
	}
}

type OnPublication func(pub *proxyproto.Publication, err error)

type ChannelStreamReader interface {
	Recv() (*proxyproto.StreamChannelResponse, error)
}

// SubscribeStream ...
func (p *Proxy) SubscribeStream(
	ctx context.Context,
	bidi bool,
	sr *proxyproto.SubscribeRequest,
	pubFunc OnPublication,
) (*proxyproto.SubscribeResponse, PublishFunc, func(), error) {
	ctx, cancel := context.WithCancel(ctx)

	var stream ChannelStreamReader

	var publishFunc PublishFunc

	if bidi {
		bidiStream, err := p.SubscribeBidirectional(ctx)
		if err != nil {
			cancel()
			return nil, nil, nil, err
		}
		err = bidiStream.Send(&proxyproto.StreamChannelRequest{
			SubscribeRequest: sr,
		})
		if err != nil {
			cancel()
			return nil, nil, nil, err
		}
		stream = bidiStream.(ChannelStreamReader)
		publishFunc = func(data []byte) error {
			return bidiStream.Send(&proxyproto.StreamChannelRequest{
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
