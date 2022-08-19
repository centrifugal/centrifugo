package client

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
	"unicode"

	"github.com/centrifugal/centrifugo/v4/internal/clientcontext"
	"github.com/centrifugal/centrifugo/v4/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v4/internal/proxy"
	"github.com/centrifugal/centrifugo/v4/internal/rule"

	"github.com/centrifugal/centrifuge"
)

// SkipUserCheckInSubscriptionToken enables mode when user id check is skipped
// while validating Subscription tokens. This flag exists for those who use
// Centrifugo v3 and want to migrate on Centrifugo v4. Eventually this flag
// should be removed – all tokens must be issued for users who initiate
// connection.
var SkipUserCheckInSubscriptionToken bool

// RPCExtensionFunc ...
type RPCExtensionFunc func(c *centrifuge.Client, e centrifuge.RPCEvent) (centrifuge.RPCReply, error)

// ProxyMap is a structure which contains all configured and already initialized
// proxies which can be used from inside client event handlers.
type ProxyMap struct {
	ConnectProxy     proxy.ConnectProxy
	RefreshProxy     proxy.RefreshProxy
	RpcProxies       map[string]proxy.RPCProxy
	PublishProxies   map[string]proxy.PublishProxy
	SubscribeProxies map[string]proxy.SubscribeProxy
}

// Handler ...
type Handler struct {
	node              *centrifuge.Node
	ruleContainer     *rule.Container
	tokenVerifier     jwtverify.Verifier
	proxyMap          *ProxyMap
	rpcExtension      map[string]RPCExtensionFunc
	granularProxyMode bool
}

// NewHandler ...
func NewHandler(
	node *centrifuge.Node,
	ruleContainer *rule.Container,
	tokenVerifier jwtverify.Verifier,
	proxyMap *ProxyMap,
	granularProxyMode bool,
) *Handler {
	return &Handler{
		node:              node,
		ruleContainer:     ruleContainer,
		tokenVerifier:     tokenVerifier,
		proxyMap:          proxyMap,
		granularProxyMode: granularProxyMode,
		rpcExtension:      make(map[string]RPCExtensionFunc),
	}
}

// SetRPCExtension ...
func (h *Handler) SetRPCExtension(method string, handler RPCExtensionFunc) {
	h.rpcExtension[method] = handler
}

// Setup event handlers.
func (h *Handler) Setup() error {
	var connectProxyHandler proxy.ConnectingHandlerFunc
	if h.proxyMap.ConnectProxy != nil {
		connectProxyHandler = proxy.NewConnectHandler(proxy.ConnectHandlerConfig{
			Proxy: h.proxyMap.ConnectProxy,
		}, h.ruleContainer).Handle(h.node)
	}

	var refreshProxyHandler proxy.RefreshHandlerFunc
	if h.proxyMap.RefreshProxy != nil {
		refreshProxyHandler = proxy.NewRefreshHandler(proxy.RefreshHandlerConfig{
			Proxy: h.proxyMap.RefreshProxy,
		}).Handle(h.node)
	}

	h.node.OnConnecting(func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		reply, err := h.OnClientConnecting(ctx, e, connectProxyHandler, refreshProxyHandler != nil)
		if err != nil {
			return centrifuge.ConnectReply{}, err
		}
		return reply, err
	})

	var rpcProxyHandler proxy.RPCHandlerFunc
	if len(h.proxyMap.RpcProxies) > 0 {
		rpcProxyHandler = proxy.NewRPCHandler(proxy.RPCHandlerConfig{
			Proxies:           h.proxyMap.RpcProxies,
			GranularProxyMode: h.granularProxyMode,
		}).Handle(h.node)
	}

	var publishProxyHandler proxy.PublishHandlerFunc
	if len(h.proxyMap.PublishProxies) > 0 {
		publishProxyHandler = proxy.NewPublishHandler(proxy.PublishHandlerConfig{
			Proxies:           h.proxyMap.PublishProxies,
			GranularProxyMode: h.granularProxyMode,
		}).Handle(h.node)
	}

	var subscribeProxyHandler proxy.SubscribeHandlerFunc
	if len(h.proxyMap.SubscribeProxies) > 0 {
		subscribeProxyHandler = proxy.NewSubscribeHandler(proxy.SubscribeHandlerConfig{
			Proxies:           h.proxyMap.SubscribeProxies,
			GranularProxyMode: h.granularProxyMode,
		}).Handle(h.node)
	}

	ruleConfig := h.ruleContainer.Config()
	usePersonalChannel := ruleConfig.UserSubscribeToPersonal
	singleConnection := ruleConfig.UserPersonalSingleConnection
	concurrency := ruleConfig.ClientConcurrency

	h.node.OnConnect(func(client *centrifuge.Client) {
		var stateMu sync.RWMutex
		var meta json.RawMessage
		if ctxMeta, ok := clientcontext.GetContextConnectionMeta(client.Context()); ok {
			meta = ctxMeta.Meta
		}

		userID := client.UserID()
		if usePersonalChannel && singleConnection && userID != "" {
			personalChannel := h.ruleContainer.PersonalChannel(userID)
			presenceStats, err := h.node.PresenceStats(personalChannel)
			if err != nil {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error calling presence stats", map[string]interface{}{"error": err.Error(), "client": client.ID(), "user": client.UserID()}))
				client.Disconnect(centrifuge.DisconnectServerError)
				return
			}
			if presenceStats.NumClients >= 2 {
				err = h.node.Disconnect(
					client.UserID(),
					centrifuge.WithCustomDisconnect(centrifuge.DisconnectConnectionLimit),
					centrifuge.WithDisconnectClientWhitelist([]string{client.ID()}),
				)
				if err != nil {
					h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error sending disconnect", map[string]interface{}{"error": err.Error(), "client": client.ID(), "user": client.UserID()}))
					client.Disconnect(centrifuge.DisconnectServerError)
					return
				}
			}
		}

		var semaphore chan struct{}
		if concurrency > 1 {
			semaphore = make(chan struct{}, concurrency)
		}

		client.OnRefresh(func(event centrifuge.RefreshEvent, cb centrifuge.RefreshCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				var pcd proxy.PerCallData
				stateMu.RLock()
				if refreshProxyHandler != nil && meta != nil {
					metaCopy := make([]byte, len(meta))
					copy(metaCopy, meta)
					pcd.Meta = metaCopy
				}
				stateMu.RUnlock()
				reply, extra, err := h.OnRefresh(client, event, refreshProxyHandler, pcd)
				if extra.Meta != nil {
					stateMu.Lock()
					meta = extra.Meta
					stateMu.Unlock()
				}
				cb(reply, err)
			})
		})

		if rpcProxyHandler != nil || len(h.rpcExtension) > 0 {
			client.OnRPC(func(event centrifuge.RPCEvent, cb centrifuge.RPCCallback) {
				h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
					var pcd proxy.PerCallData
					stateMu.RLock()
					if rpcProxyHandler != nil && meta != nil {
						metaCopy := make([]byte, len(meta))
						copy(metaCopy, meta)
						pcd.Meta = metaCopy
					}
					stateMu.RUnlock()
					reply, err := h.OnRPC(client, event, rpcProxyHandler, pcd)
					cb(reply, err)
				})
			})
		}

		client.OnSubscribe(func(event centrifuge.SubscribeEvent, cb centrifuge.SubscribeCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				var pcd proxy.PerCallData
				stateMu.RLock()
				if subscribeProxyHandler != nil && meta != nil {
					metaCopy := make([]byte, len(meta))
					copy(metaCopy, meta)
					pcd.Meta = metaCopy
				}
				stateMu.RUnlock()
				reply, _, err := h.OnSubscribe(client, event, subscribeProxyHandler, pcd)
				cb(reply, err)
			})
		})

		client.OnSubRefresh(func(event centrifuge.SubRefreshEvent, cb centrifuge.SubRefreshCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, _, err := h.OnSubRefresh(client, event)
				cb(reply, err)
			})
		})

		client.OnPublish(func(event centrifuge.PublishEvent, cb centrifuge.PublishCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				var pcd proxy.PerCallData
				stateMu.RLock()
				if publishProxyHandler != nil && meta != nil {
					metaCopy := make([]byte, len(meta))
					copy(metaCopy, meta)
					pcd.Meta = metaCopy
				}
				stateMu.RUnlock()
				reply, err := h.OnPublish(client, event, publishProxyHandler, pcd)
				cb(reply, err)
			})
		})

		client.OnPresence(func(event centrifuge.PresenceEvent, cb centrifuge.PresenceCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, err := h.OnPresence(client, event)
				cb(reply, err)
			})
		})

		client.OnPresenceStats(func(event centrifuge.PresenceStatsEvent, cb centrifuge.PresenceStatsCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, err := h.OnPresenceStats(client, event)
				cb(reply, err)
			})
		})

		client.OnHistory(func(event centrifuge.HistoryEvent, cb centrifuge.HistoryCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, err := h.OnHistory(client, event)
				cb(reply, err)
			})
		})
	})
	return nil
}

func (h *Handler) runConcurrentlyIfNeeded(ctx context.Context, concurrency int, semaphore chan struct{}, fn func()) {
	if concurrency > 1 {
		select {
		case <-ctx.Done():
			return
		case semaphore <- struct{}{}:
		}
		go func() {
			defer func() { <-semaphore }()
			fn()
		}()
	} else {
		fn()
	}
}

// OnClientConnecting ...
func (h *Handler) OnClientConnecting(
	ctx context.Context,
	e centrifuge.ConnectEvent,
	connectProxyHandler proxy.ConnectingHandlerFunc,
	refreshProxyEnabled bool,
) (centrifuge.ConnectReply, error) {
	var (
		credentials *centrifuge.Credentials
		data        []byte
		newCtx      context.Context
	)

	subscriptions := make(map[string]centrifuge.SubscribeOptions)

	ruleConfig := h.ruleContainer.Config()

	var processClientChannels bool

	if e.Token != "" {
		token, err := h.tokenVerifier.VerifyConnectToken(e.Token)
		if err != nil {
			if err == jwtverify.ErrTokenExpired {
				return centrifuge.ConnectReply{}, centrifuge.ErrorTokenExpired
			}
			if errors.Is(err, jwtverify.ErrInvalidToken) {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid connection token", map[string]interface{}{"error": err.Error(), "client": e.ClientID}))
				return centrifuge.ConnectReply{}, centrifuge.DisconnectInvalidToken
			}
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "internal server error", map[string]interface{}{"error": err.Error(), "client": e.ClientID}))
			return centrifuge.ConnectReply{}, err
		}

		credentials = &centrifuge.Credentials{
			UserID:   token.UserID,
			ExpireAt: token.ExpireAt,
			Info:     token.Info,
		}

		if ruleConfig.ClientInsecure {
			credentials.ExpireAt = 0
		}

		subscriptions = token.Subs

		if token.Meta != nil {
			newCtx = clientcontext.SetContextConnectionMeta(ctx, clientcontext.ConnectionMeta{
				Meta: token.Meta,
			})
		}

		processClientChannels = true
	} else if connectProxyHandler != nil {
		connectReply, _, err := connectProxyHandler(ctx, e)
		if err != nil {
			return centrifuge.ConnectReply{}, err
		}
		credentials = connectReply.Credentials
		if connectReply.Subscriptions != nil {
			subscriptions = connectReply.Subscriptions
		}
		data = connectReply.Data
		newCtx = connectReply.Context
	}

	// Proceed with Credentials with empty user ID in case anonymous or insecure options on.
	if credentials == nil && (ruleConfig.AnonymousConnectWithoutToken || ruleConfig.ClientInsecure) {
		credentials = &centrifuge.Credentials{
			UserID: "",
		}
	}

	// Automatically subscribe on personal server-side channel.
	if credentials != nil && ruleConfig.UserSubscribeToPersonal && credentials.UserID != "" {
		personalChannel := h.ruleContainer.PersonalChannel(credentials.UserID)
		_, _, chOpts, found, err := h.ruleContainer.ChannelOptions(personalChannel)
		if err != nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "subscribe channel options error", map[string]interface{}{"error": err.Error(), "channel": personalChannel}))
			return centrifuge.ConnectReply{}, err
		}
		if !found {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe unknown personal channel", map[string]interface{}{"channel": personalChannel}))
			return centrifuge.ConnectReply{}, centrifuge.ErrorUnknownChannel
		}
		subscriptions[personalChannel] = centrifuge.SubscribeOptions{
			EmitPresence:      chOpts.Presence,
			EmitJoinLeave:     chOpts.JoinLeave,
			PushJoinLeave:     chOpts.ForcePushJoinLeave,
			EnableRecovery:    chOpts.ForceRecovery,
			EnablePositioning: chOpts.ForcePositioning,
		}
	}

	if processClientChannels {
		// Try to satisfy client request regarding desired server-side subscriptions.
		// Subscribe only to channels client has permission to, so that it could theoretically
		// just use client-side subscriptions to achieve the same.
		for _, ch := range e.Channels {
			nsName, rest, chOpts, found, err := h.ruleContainer.ChannelOptions(ch)
			if err != nil {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "channel options error", map[string]interface{}{"error": err.Error(), "channel": ch}))
				return centrifuge.ConnectReply{}, err
			}
			if !found {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe unknown channel", map[string]interface{}{"channel": ch}))
				return centrifuge.ConnectReply{}, centrifuge.DisconnectBadRequest
			}

			var channelOk bool

			isUserLimited := h.ruleContainer.IsUserLimited(ch)
			isPrivate := h.ruleContainer.IsPrivateChannel(ch)

			var userID string
			if credentials != nil {
				userID = credentials.UserID
			}

			validChannelName, err := h.validChannelName(nsName, rest, chOpts, ch)
			if err != nil {
				return centrifuge.ConnectReply{}, centrifuge.ErrorInternal
			}

			if !isPrivate && !chOpts.ProxySubscribe && chOpts.SubscribeProxyName == "" && validChannelName {
				if isUserLimited && chOpts.UserLimitedChannels && (userID != "" && h.ruleContainer.UserAllowed(ch, userID)) {
					channelOk = true
				} else if chOpts.SubscribeForClient && (userID != "" || chOpts.SubscribeForAnonymous) {
					channelOk = true
				}
			}

			if channelOk {
				if _, ok := subscriptions[ch]; !ok {
					subscriptions[ch] = centrifuge.SubscribeOptions{
						EmitPresence:      chOpts.Presence,
						EmitJoinLeave:     chOpts.JoinLeave,
						PushJoinLeave:     chOpts.ForcePushJoinLeave,
						EnableRecovery:    chOpts.ForceRecovery,
						EnablePositioning: chOpts.ForcePositioning,
					}
				}
			} else {
				if h.node.LogEnabled(centrifuge.LogLevelDebug) {
					h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "ignoring subscription to a channel", map[string]interface{}{"channel": ch, "client": e.ClientID, "user": userID}))
				}
			}
		}
	}

	finalReply := centrifuge.ConnectReply{
		Credentials:       credentials,
		Subscriptions:     subscriptions,
		Data:              data,
		ClientSideRefresh: !refreshProxyEnabled,
	}
	if newCtx != nil {
		finalReply.Context = newCtx
	}
	return finalReply, nil
}

type RefreshExtra struct {
	Meta json.RawMessage
}

// OnRefresh ...
func (h *Handler) OnRefresh(c *centrifuge.Client, e centrifuge.RefreshEvent, refreshProxyHandler proxy.RefreshHandlerFunc, d proxy.PerCallData) (centrifuge.RefreshReply, RefreshExtra, error) {
	if refreshProxyHandler != nil {
		r, extra, err := refreshProxyHandler(c, e, d)
		return r, RefreshExtra{Meta: extra.Meta}, err
	}
	token, err := h.tokenVerifier.VerifyConnectToken(e.Token)
	if err != nil {
		if err == jwtverify.ErrTokenExpired {
			return centrifuge.RefreshReply{Expired: true}, RefreshExtra{}, nil
		}
		if errors.Is(err, jwtverify.ErrInvalidToken) {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid refresh token", map[string]interface{}{"error": err.Error(), "user": c.UserID(), "client": c.ID()}))
			return centrifuge.RefreshReply{}, RefreshExtra{}, centrifuge.DisconnectInvalidToken
		}
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error verifying refresh token", map[string]interface{}{"error": err.Error(), "user": c.UserID(), "client": c.ID()}))
		return centrifuge.RefreshReply{}, RefreshExtra{}, err
	}
	if token.UserID != c.UserID() {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "refresh token user mismatch", map[string]interface{}{"tokenUser": token.UserID, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.RefreshReply{}, RefreshExtra{}, centrifuge.DisconnectInvalidToken
	}
	return centrifuge.RefreshReply{
		ExpireAt: token.ExpireAt,
		Info:     token.Info,
	}, RefreshExtra{Meta: token.Meta}, nil
}

// OnRPC ...
func (h *Handler) OnRPC(c *centrifuge.Client, e centrifuge.RPCEvent, rpcProxyHandler proxy.RPCHandlerFunc, d proxy.PerCallData) (centrifuge.RPCReply, error) {
	if handler, ok := h.rpcExtension[e.Method]; ok {
		return handler(c, e)
	}
	if rpcProxyHandler != nil {
		return rpcProxyHandler(c, e, h.ruleContainer, d)
	}
	return centrifuge.RPCReply{}, centrifuge.ErrorMethodNotFound
}

type SubRefreshExtra struct {
}

// OnSubRefresh ...
func (h *Handler) OnSubRefresh(c *centrifuge.Client, e centrifuge.SubRefreshEvent) (centrifuge.SubRefreshReply, SubRefreshExtra, error) {
	token, err := h.tokenVerifier.VerifySubscribeToken(e.Token)
	if err != nil {
		if err == jwtverify.ErrTokenExpired {
			return centrifuge.SubRefreshReply{Expired: true}, SubRefreshExtra{}, nil
		}
		if errors.Is(err, jwtverify.ErrInvalidToken) {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid subscription refresh token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectInvalidToken
		}
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error verifying subscription refresh token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, err
	}
	if e.Channel == "" {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "sub refresh empty channel", map[string]interface{}{"tokenChannel": token.Channel, "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectInvalidToken
	}
	if e.Channel != token.Channel {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "sub refresh token channel mismatch", map[string]interface{}{"channel": e.Channel, "tokenChannel": token.Channel, "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectInvalidToken
	}
	if !SkipUserCheckInSubscriptionToken && token.UserID != c.UserID() {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "sub refresh token user mismatch", map[string]interface{}{"channel": e.Channel, "tokenUser": token.UserID, "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectInvalidToken
	}
	if token.Client != "" && c.ID() != token.Client {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "token client mismatch", map[string]interface{}{"channel": e.Channel, "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectInvalidToken
	}
	return centrifuge.SubRefreshReply{
		ExpireAt: token.Options.ExpireAt,
		Info:     token.Options.ChannelInfo,
	}, SubRefreshExtra{}, nil
}

func isASCII(s string) bool {
	for _, c := range s {
		if c > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func (h *Handler) validChannelName(_ string, rest string, chOpts rule.ChannelOptions, channel string) (bool, error) {
	if chOpts.ChannelRegex != "" {
		regex := chOpts.Compiled.CompiledChannelRegex
		if !regex.MatchString(rest) {
			return false, nil
		}
	} else if !isASCII(channel) {
		return false, nil
	}
	return true, nil
}

func (h *Handler) validateChannelName(c *centrifuge.Client, nsName string, rest string, chOpts rule.ChannelOptions, channel string) error {
	ok, err := h.validChannelName(nsName, rest, chOpts, channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "error checking channel name", map[string]interface{}{"channel": channel, "error": err.Error(), "client": c.ID(), "user": c.UserID()}))
		return centrifuge.ErrorInternal
	}
	if !ok {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid channel name", map[string]interface{}{"channel": channel, "client": c.ID(), "user": c.UserID()}))
		return centrifuge.ErrorBadRequest
	}
	return nil
}

type SubscribeExtra struct {
}

// OnSubscribe ...
func (h *Handler) OnSubscribe(c *centrifuge.Client, e centrifuge.SubscribeEvent, subscribeProxyHandler proxy.SubscribeHandlerFunc, d proxy.PerCallData) (centrifuge.SubscribeReply, SubscribeExtra, error) {
	ruleConfig := h.ruleContainer.Config()

	if e.Channel == "" {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe empty channel", map[string]interface{}{"user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorUnknownChannel
	}

	nsName, rest, chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "subscribe channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorUnknownChannel
	}
	if err = h.validateChannelName(c, nsName, rest, chOpts, e.Channel); err != nil {
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, err
	}

	var allowed bool

	var options centrifuge.SubscribeOptions

	options.EmitPresence = chOpts.Presence
	options.EmitJoinLeave = chOpts.JoinLeave
	options.PushJoinLeave = chOpts.ForcePushJoinLeave
	options.EnablePositioning = chOpts.ForcePositioning
	options.EnableRecovery = chOpts.ForceRecovery

	if h.ruleContainer.IsPrivateChannel(e.Channel) && e.Token == "" {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "attempt to subscribe on private channel without token", map[string]interface{}{"channel": e.Channel, "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorPermissionDenied
	}

	if e.Token != "" {
		token, err := h.tokenVerifier.VerifySubscribeToken(e.Token)
		if err != nil {
			if err == jwtverify.ErrTokenExpired {
				return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorTokenExpired
			}
			if errors.Is(err, jwtverify.ErrInvalidToken) {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid subscription token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
				return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorPermissionDenied
			}
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error verifying subscription token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, err
		}
		if e.Channel != token.Channel {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "token channel mismatch", map[string]interface{}{"channel": e.Channel, "tokenChannel": token.Channel, "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.DisconnectInvalidToken
		}
		if !SkipUserCheckInSubscriptionToken && token.UserID != c.UserID() {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "token user mismatch", map[string]interface{}{"channel": e.Channel, "tokenUser": token.UserID, "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.DisconnectInvalidToken
		}
		if token.Client != "" && c.ID() != token.Client {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "token client mismatch", map[string]interface{}{"channel": e.Channel, "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.DisconnectInvalidToken
		}
		options = token.Options
		allowed = true
	} else if chOpts.ProxySubscribe || chOpts.SubscribeProxyName != "" {
		if subscribeProxyHandler == nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe proxy not enabled", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorNotAvailable
		}
		r, _, err := subscribeProxyHandler(c, e, chOpts, d)
		return r, SubscribeExtra{}, err
	} else if chOpts.UserLimitedChannels && h.ruleContainer.IsUserLimited(e.Channel) && h.ruleContainer.UserAllowed(e.Channel, c.UserID()) {
		allowed = true
	} else if chOpts.SubscribeForClient && (c.UserID() != "" || chOpts.SubscribeForAnonymous) {
		allowed = true
	} else if ruleConfig.ClientInsecure {
		allowed = true
	}

	if e.Positioned && (chOpts.AllowPositioning || h.hasAccessToHistory(c, e.Channel, chOpts)) {
		options.EnablePositioning = true
	}
	if e.Recoverable && (chOpts.AllowRecovery || h.hasAccessToHistory(c, e.Channel, chOpts)) {
		options.EnableRecovery = true
	}
	if e.JoinLeave && chOpts.JoinLeave && h.hasAccessToPresence(c, e.Channel, chOpts) {
		options.PushJoinLeave = true
	}

	if !allowed {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to subscribe without sufficient permission", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorPermissionDenied
	}

	return centrifuge.SubscribeReply{
		Options:           options,
		ClientSideRefresh: true,
	}, SubscribeExtra{}, nil
}

// OnPublish ...
func (h *Handler) OnPublish(c *centrifuge.Client, e centrifuge.PublishEvent, publishProxyHandler proxy.PublishHandlerFunc, d proxy.PerCallData) (centrifuge.PublishReply, error) {
	ruleConfig := h.ruleContainer.Config()

	nsName, rest, chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "publish channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PublishReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "publish to unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PublishReply{}, centrifuge.ErrorUnknownChannel
	}
	if err = h.validateChannelName(c, nsName, rest, chOpts, e.Channel); err != nil {
		return centrifuge.PublishReply{}, err
	}

	var allowed bool

	if chOpts.ProxyPublish || chOpts.PublishProxyName != "" {
		if publishProxyHandler == nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "publish proxy not enabled", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
			return centrifuge.PublishReply{}, centrifuge.ErrorNotAvailable
		}
		return publishProxyHandler(c, e, chOpts, d)
	} else if chOpts.PublishForClient && (c.UserID() != "" || chOpts.PublishForAnonymous) {
		allowed = true
	} else if chOpts.PublishForSubscriber && c.IsSubscribed(e.Channel) && (c.UserID() != "" || chOpts.PublishForAnonymous) {
		allowed = true
	} else if ruleConfig.ClientInsecure {
		allowed = true
	}

	if !allowed {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to publish without sufficient permission", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PublishReply{}, centrifuge.ErrorPermissionDenied
	}

	result, err := h.node.Publish(
		e.Channel, e.Data,
		centrifuge.WithClientInfo(e.ClientInfo),
		centrifuge.WithHistory(chOpts.HistorySize, time.Duration(chOpts.HistoryTTL)),
	)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "publish error", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID(), "error": err.Error()}))
	}
	return centrifuge.PublishReply{Result: &result}, err
}

func (h *Handler) hasAccessToPresence(c *centrifuge.Client, channel string, chOpts rule.ChannelOptions) bool {
	if chOpts.PresenceForClient && (c.UserID() != "" || chOpts.PresenceForAnonymous) {
		return true
	} else if chOpts.PresenceForSubscriber && c.IsSubscribed(channel) && (c.UserID() != "" || chOpts.PresenceForAnonymous) {
		return true
	} else if h.ruleContainer.Config().ClientInsecure {
		return true
	}
	return false
}

// OnPresence ...
func (h *Handler) OnPresence(c *centrifuge.Client, e centrifuge.PresenceEvent) (centrifuge.PresenceReply, error) {
	nsName, rest, chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "presence channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "presence for unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceReply{}, centrifuge.ErrorUnknownChannel
	}
	if err = h.validateChannelName(c, nsName, rest, chOpts, e.Channel); err != nil {
		return centrifuge.PresenceReply{}, err
	}
	if !chOpts.Presence {
		return centrifuge.PresenceReply{}, centrifuge.ErrorNotAvailable
	}

	allowed := h.hasAccessToPresence(c, e.Channel, chOpts)

	if !allowed {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to call presence without sufficient permission", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceReply{}, centrifuge.ErrorPermissionDenied
	}

	return centrifuge.PresenceReply{}, nil
}

// OnPresenceStats ...
func (h *Handler) OnPresenceStats(c *centrifuge.Client, e centrifuge.PresenceStatsEvent) (centrifuge.PresenceStatsReply, error) {
	nsName, rest, chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "presence stats channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceStatsReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "presence stats for unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceStatsReply{}, centrifuge.ErrorUnknownChannel
	}
	if err = h.validateChannelName(c, nsName, rest, chOpts, e.Channel); err != nil {
		return centrifuge.PresenceStatsReply{}, err
	}
	if !chOpts.Presence {
		return centrifuge.PresenceStatsReply{}, centrifuge.ErrorNotAvailable
	}

	allowed := h.hasAccessToPresence(c, e.Channel, chOpts)

	if !allowed {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to call presence stats without sufficient permission", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceStatsReply{}, centrifuge.ErrorPermissionDenied
	}

	return centrifuge.PresenceStatsReply{}, nil
}

func (h *Handler) hasAccessToHistory(c *centrifuge.Client, channel string, chOpts rule.ChannelOptions) bool {
	if chOpts.HistoryForClient && (c.UserID() != "" || chOpts.HistoryForAnonymous) {
		return true
	} else if chOpts.HistoryForSubscriber && c.IsSubscribed(channel) && (c.UserID() != "" || chOpts.HistoryForAnonymous) {
		return true
	} else if h.ruleContainer.Config().ClientInsecure {
		return true
	}
	return false
}

// OnHistory ...
func (h *Handler) OnHistory(c *centrifuge.Client, e centrifuge.HistoryEvent) (centrifuge.HistoryReply, error) {
	nsName, rest, chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "history channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.HistoryReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "history for unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.HistoryReply{}, centrifuge.ErrorUnknownChannel
	}
	if err = h.validateChannelName(c, nsName, rest, chOpts, e.Channel); err != nil {
		return centrifuge.HistoryReply{}, err
	}
	if chOpts.HistorySize <= 0 || chOpts.HistoryTTL <= 0 {
		return centrifuge.HistoryReply{}, centrifuge.ErrorNotAvailable
	}

	allowed := h.hasAccessToHistory(c, e.Channel, chOpts)

	if !allowed {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to call history without sufficient permission", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.HistoryReply{}, centrifuge.ErrorPermissionDenied
	}

	return centrifuge.HistoryReply{}, nil
}
