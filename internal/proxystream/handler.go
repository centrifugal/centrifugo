package proxystream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/clientstorage"
	"github.com/centrifugal/centrifugo/v5/internal/proxystreamproto"
	"github.com/centrifugal/centrifugo/v5/internal/rule"
	"github.com/centrifugal/centrifugo/v5/internal/subsource"

	"github.com/centrifugal/centrifuge"
)

// HandlerConfig ...
type HandlerConfig struct {
	ConnectProxy         *Proxy
	ConnectBidirectional bool
	SubscribeProxies     map[string]*Proxy
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

type SendFunc func(data []byte) error

// ConnectHandlerFunc ...
type ConnectHandlerFunc func(context.Context, centrifuge.ConnectEvent) (centrifuge.ConnectReply, SendFunc, error)

// HandleConnect ...
func (h *Handler) HandleConnect(node *centrifuge.Node) ConnectHandlerFunc {
	return func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, SendFunc, error) {
		proxyName := "__default__"
		proxyCallTotalCount.WithLabelValues(proxyName, "connect").Inc()

		p := h.config.ConnectProxy // Granular proxy mode not yet available for stream proxies, we only have global one.

		req := &proxystreamproto.ConnectRequest{
			Client:    e.ClientID,
			Protocol:  string(e.Transport.Protocol()),
			Transport: e.Transport.Name(),

			Name:    e.Name,
			Version: e.Version,
			Data:    e.Data,
		}

		clientReady := make(chan *centrifuge.Client, 1)
		var clientReadyOnce sync.Once

		var c Client

		connectRep, sendFunc, err := p.ConnectStream(ctx, h.config.ConnectBidirectional, req, func(msg *proxystreamproto.Message, err error) {
			clientReadyOnce.Do(func() {
				select {
				case c = <-clientReady:
				case <-ctx.Done():
					return
				}
			})
			select {
			case <-ctx.Done():
				return
			default:
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					c.Disconnect(centrifuge.DisconnectForceNoReconnect)
					return
				}
				c.Disconnect(centrifuge.DisconnectInsufficientState)
				return
			}
			_ = c.Send(msg.Data)
		})
		if err != nil {
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error from connect stream proxy", map[string]any{"error": err.Error()}))
			proxyCallErrorCount.WithLabelValues(proxyName, "connect", "internal").Inc()
			return centrifuge.ConnectReply{}, nil, err
		}

		if connectRep.Disconnect != nil {
			proxyCallErrorCount.WithLabelValues(proxyName, "connect", "disconnect_"+strconv.FormatUint(uint64(connectRep.Disconnect.Code), 10)).Inc()
			return centrifuge.ConnectReply{}, nil, &centrifuge.Disconnect{
				Code:   connectRep.Disconnect.Code,
				Reason: connectRep.Disconnect.Reason,
			}
		}
		if connectRep.Error != nil {
			proxyCallErrorCount.WithLabelValues(proxyName, "connect", "error_"+strconv.FormatUint(uint64(connectRep.Error.Code), 10)).Inc()
			return centrifuge.ConnectReply{}, nil, &centrifuge.Error{
				Code:    connectRep.Error.Code,
				Message: connectRep.Error.Message,
			}
		}

		result := connectRep.Result
		if result == nil {
			return centrifuge.ConnectReply{Credentials: nil}, nil, nil
		}
		info := result.Info
		data := result.Data

		reply := centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: result.User,
				Info:   info,
			},
		}
		if len(data) > 0 {
			reply.Data = data
		}

		if result.Meta != nil {
			reply.Storage = map[string]any{
				clientstorage.KeyMeta: json.RawMessage(result.Meta),
			}
		}

		reply.ClientReady = clientReady

		return reply, sendFunc, nil
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

		req := &proxystreamproto.SubscribeRequest{
			Client:    c.ID(),
			Protocol:  string(c.Transport().Protocol()),
			Transport: c.Transport().Name(),

			User:    c.UserID(),
			Channel: e.Channel,
		}
		req.Data = e.Data
		if p.config.IncludeConnectionMeta && pcd.Meta != nil {
			req.Meta = pcd.Meta
		}

		subscriptionReady := make(chan struct{})
		var subscriptionReadyOnce sync.Once

		subscribeRep, publishFunc, cancelFunc, err := p.SubscribeStream(c.Context(), bidi, req, func(pub *proxystreamproto.Publication, err error) {
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

type OnMessage func(msg *proxystreamproto.Message, err error)

type StreamReader interface {
	Recv() (*proxystreamproto.Response, error)
}

// ConnectStream ...
func (p *Proxy) ConnectStream(
	ctx context.Context,
	bidi bool,
	sr *proxystreamproto.ConnectRequest,
	onMessage OnMessage,
) (*proxystreamproto.ConnectResponse, SendFunc, error) {
	ctx, cancel := context.WithCancel(ctx)

	var stream StreamReader

	var sendFunc SendFunc

	if bidi {
		bidiStream, err := p.ConnectBidirectional(ctx)
		if err != nil {
			cancel()
			return nil, nil, err
		}
		err = bidiStream.Send(&proxystreamproto.Request{
			ConnectRequest: sr,
		})
		if err != nil {
			cancel()
			return nil, nil, err
		}
		stream = bidiStream.(StreamReader)
		sendFunc = func(data []byte) error {
			return bidiStream.Send(&proxystreamproto.Request{
				Message: &proxystreamproto.Message{
					Data: data,
				},
			})
		}
	} else {
		var err error
		stream, err = p.ConnectUnidirectional(ctx, sr)
		if err != nil {
			cancel()
			return nil, nil, err
		}
	}

	firstMessageReceived := make(chan struct{})

	go func() {
		ticker := time.NewTicker(time.Duration(p.config.Timeout))
		defer ticker.Stop()
		select {
		case <-ctx.Done():
			cancel()
			return
		case <-ticker.C:
			cancel()
			return
		case <-firstMessageReceived:
		}
	}()

	resp, err := stream.Recv()
	if err != nil {
		cancel()
		return nil, nil, err
	}
	close(firstMessageReceived)

	go func() {
		for {
			msgResp, err := stream.Recv()
			if err != nil {
				cancel()
				onMessage(nil, err)
				return
			}
			msg := msgResp.GetMessage()
			if msg != nil {
				// TODO: better handling of unexpected nil publication.
				onMessage(msg, nil)
			}
		}
	}()

	if resp.ConnectResponse == nil {
		return nil, nil, errors.New("no connect response in first message from stream proxy")
	}
	return resp.ConnectResponse, sendFunc, nil
}

type OnPublication func(pub *proxystreamproto.Publication, err error)

type ChannelStreamReader interface {
	Recv() (*proxystreamproto.ChannelResponse, error)
}

// SubscribeStream ...
func (p *Proxy) SubscribeStream(
	ctx context.Context,
	bidi bool,
	sr *proxystreamproto.SubscribeRequest,
	pubFunc OnPublication,
) (*proxystreamproto.SubscribeResponse, PublishFunc, func(), error) {
	ctx, cancel := context.WithCancel(ctx)

	var stream ChannelStreamReader

	var publishFunc PublishFunc

	if bidi {
		bidiStream, err := p.SubscribeBidirectional(ctx)
		if err != nil {
			cancel()
			return nil, nil, nil, err
		}
		err = bidiStream.Send(&proxystreamproto.ChannelRequest{
			SubscribeRequest: sr,
		})
		if err != nil {
			cancel()
			return nil, nil, nil, err
		}
		stream = bidiStream.(ChannelStreamReader)
		publishFunc = func(data []byte) error {
			return bidiStream.Send(&proxystreamproto.ChannelRequest{
				Publication: &proxystreamproto.Publication{
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
