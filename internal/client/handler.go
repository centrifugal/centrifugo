package client

import (
	"context"
	"errors"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/clientcontext"
	"github.com/centrifugal/centrifugo/v3/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v3/internal/proxy"
	"github.com/centrifugal/centrifugo/v3/internal/rule"

	"github.com/centrifugal/centrifuge"
)

// UseUnlimitedHistoryByDefault enables mode when requests without limit set
// will cause calling history with unlimited option on. This is done to help
// migrating clients before switching to new history API entirely. This toggle
// should be removed at some point during Centrifugo v3 life cycle.
var UseUnlimitedHistoryByDefault bool

// RPCExtensionFunc ...
type RPCExtensionFunc func(c *centrifuge.Client, e centrifuge.RPCEvent) (centrifuge.RPCReply, error)

// Handler ...
type Handler struct {
	node          *centrifuge.Node
	ruleContainer *rule.Container
	tokenVerifier jwtverify.Verifier
	proxyConfig   proxy.Config
	rpcExtension  map[string]RPCExtensionFunc
}

// NewHandler ...
func NewHandler(
	node *centrifuge.Node,
	ruleContainer *rule.Container,
	tokenVerifier jwtverify.Verifier,
	proxyConfig proxy.Config,
) *Handler {
	return &Handler{
		node:          node,
		ruleContainer: ruleContainer,
		tokenVerifier: tokenVerifier,
		proxyConfig:   proxyConfig,
		rpcExtension:  make(map[string]RPCExtensionFunc),
	}
}

// SetRPCExtension ...
func (h *Handler) SetRPCExtension(method string, handler RPCExtensionFunc) {
	h.rpcExtension[method] = handler
}

// Setup event handlers.
func (h *Handler) Setup() error {
	var connectProxyHandler centrifuge.ConnectingHandler
	if h.proxyConfig.ConnectEndpoint != "" {
		connectProxy, err := h.getConnectProxy()
		if err != nil {
			return err
		}
		connectProxyHandler = proxy.NewConnectHandler(proxy.ConnectHandlerConfig{
			Proxy: connectProxy,
		}, h.ruleContainer).Handle(h.node)
	}

	var refreshProxyHandler proxy.RefreshHandlerFunc
	if h.proxyConfig.RefreshEndpoint != "" {
		refreshProxy, err := h.getRefreshProxy()
		if err != nil {
			return err
		}
		refreshProxyHandler = proxy.NewRefreshHandler(proxy.RefreshHandlerConfig{
			Proxy: refreshProxy,
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
	if h.proxyConfig.RPCEndpoint != "" {
		rpcProxy, err := h.getRPCProxy()
		if err != nil {
			return err
		}
		rpcProxyHandler = proxy.NewRPCHandler(proxy.RPCHandlerConfig{
			Proxy: rpcProxy,
		}).Handle(h.node)
	}

	var publishProxyHandler proxy.PublishHandlerFunc
	if h.proxyConfig.PublishEndpoint != "" {
		publishProxy, err := h.getPublishProxy()
		if err != nil {
			return err
		}
		publishProxyHandler = proxy.NewPublishHandler(proxy.PublishHandlerConfig{
			Proxy: publishProxy,
		}).Handle(h.node)
	}

	var subscribeProxyHandler proxy.SubscribeHandlerFunc
	if h.proxyConfig.SubscribeEndpoint != "" {
		subscribeProxy, err := h.getSubscribeProxy()
		if err != nil {
			return err
		}
		subscribeProxyHandler = proxy.NewSubscribeHandler(proxy.SubscribeHandlerConfig{
			Proxy: subscribeProxy,
		}).Handle(h.node)
	}

	ruleConfig := h.ruleContainer.Config()
	usePersonalChannel := ruleConfig.UserSubscribeToPersonal
	singleConnection := ruleConfig.UserPersonalSingleConnection
	concurrency := ruleConfig.ClientConcurrency

	h.node.OnConnect(func(client *centrifuge.Client) {
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
					centrifuge.WithDisconnect(centrifuge.DisconnectConnectionLimit),
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
			h.runConcurrentlyIfNeeded(concurrency, semaphore, func() {
				reply, err := h.OnRefresh(client, event, refreshProxyHandler)
				cb(reply, err)
			})
		})

		if rpcProxyHandler != nil || len(h.rpcExtension) > 0 {
			client.OnRPC(func(event centrifuge.RPCEvent, cb centrifuge.RPCCallback) {
				h.runConcurrentlyIfNeeded(concurrency, semaphore, func() {
					reply, err := h.OnRPC(client, event, rpcProxyHandler)
					cb(reply, err)
				})
			})
		}

		client.OnSubscribe(func(event centrifuge.SubscribeEvent, cb centrifuge.SubscribeCallback) {
			h.runConcurrentlyIfNeeded(concurrency, semaphore, func() {
				reply, err := h.OnSubscribe(client, event, subscribeProxyHandler)
				cb(reply, err)
			})
		})

		client.OnSubRefresh(func(event centrifuge.SubRefreshEvent, cb centrifuge.SubRefreshCallback) {
			h.runConcurrentlyIfNeeded(concurrency, semaphore, func() {
				reply, err := h.OnSubRefresh(client, event)
				cb(reply, err)
			})
		})

		client.OnPublish(func(event centrifuge.PublishEvent, cb centrifuge.PublishCallback) {
			h.runConcurrentlyIfNeeded(concurrency, semaphore, func() {
				reply, err := h.OnPublish(client, event, publishProxyHandler)
				cb(reply, err)
			})
		})

		client.OnPresence(func(event centrifuge.PresenceEvent, cb centrifuge.PresenceCallback) {
			h.runConcurrentlyIfNeeded(concurrency, semaphore, func() {
				reply, err := h.OnPresence(client, event)
				cb(reply, err)
			})
		})

		client.OnPresenceStats(func(event centrifuge.PresenceStatsEvent, cb centrifuge.PresenceStatsCallback) {
			h.runConcurrentlyIfNeeded(concurrency, semaphore, func() {
				reply, err := h.OnPresenceStats(client, event)
				cb(reply, err)
			})
		})

		client.OnHistory(func(event centrifuge.HistoryEvent, cb centrifuge.HistoryCallback) {
			h.runConcurrentlyIfNeeded(concurrency, semaphore, func() {
				reply, err := h.OnHistory(client, event)
				cb(reply, err)
			})
		})
	})
	return nil
}

func (h *Handler) runConcurrentlyIfNeeded(concurrency int, semaphore chan struct{}, fn func()) {
	if concurrency > 1 {
		semaphore <- struct{}{}
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
	connectProxyHandler centrifuge.ConnectingHandler,
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
		connectReply, err := connectProxyHandler(ctx, e)
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
	if credentials == nil && (ruleConfig.ClientAnonymous || ruleConfig.ClientInsecure) {
		credentials = &centrifuge.Credentials{
			UserID: "",
		}
	}

	// Automatically subscribe on personal server-side channel.
	if credentials != nil && ruleConfig.UserSubscribeToPersonal && credentials.UserID != "" {
		personalChannel := h.ruleContainer.PersonalChannel(credentials.UserID)
		chOpts, found, err := h.ruleContainer.ChannelOptions(personalChannel)
		if err != nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "subscribe channel options error", map[string]interface{}{"error": err.Error(), "channel": personalChannel}))
			return centrifuge.ConnectReply{}, err
		}
		if !found {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe unknown personal channel", map[string]interface{}{"channel": personalChannel}))
			return centrifuge.ConnectReply{}, centrifuge.ErrorUnknownChannel
		}
		subscriptions[personalChannel] = centrifuge.SubscribeOptions{
			Presence:  chOpts.Presence,
			JoinLeave: chOpts.JoinLeave,
			Recover:   chOpts.Recover,
		}
	}

	if processClientChannels {
		// Try to satisfy client request regarding desired server-side subscriptions.
		// Subscribe only on allowed user-limited channels, ignore private channels,
		// ignore channels from protected namespaces.
		for _, ch := range e.Channels {
			chOpts, found, err := h.ruleContainer.ChannelOptions(ch)
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

			if isUserLimited {
				if userID != "" && h.ruleContainer.UserAllowed(ch, userID) {
					channelOk = true
				}
			} else if !isPrivate && !chOpts.Protected {
				channelOk = true
			}

			if channelOk {
				if _, ok := subscriptions[ch]; !ok {
					subscriptions[ch] = centrifuge.SubscribeOptions{
						Presence:  chOpts.Presence,
						JoinLeave: chOpts.JoinLeave,
						Recover:   chOpts.Recover,
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

// OnRefresh ...
func (h *Handler) OnRefresh(c *centrifuge.Client, e centrifuge.RefreshEvent, refreshProxyHandler proxy.RefreshHandlerFunc) (centrifuge.RefreshReply, error) {
	if refreshProxyHandler != nil {
		return refreshProxyHandler(c, e)
	}
	token, err := h.tokenVerifier.VerifyConnectToken(e.Token)
	if err != nil {
		if err == jwtverify.ErrTokenExpired {
			return centrifuge.RefreshReply{Expired: true}, nil
		}
		if errors.Is(err, jwtverify.ErrInvalidToken) {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid refresh token", map[string]interface{}{"error": err.Error(), "user": c.UserID(), "client": c.ID()}))
			return centrifuge.RefreshReply{}, centrifuge.DisconnectInvalidToken
		}
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error verifying refresh token", map[string]interface{}{"error": err.Error(), "user": c.UserID(), "client": c.ID()}))
		return centrifuge.RefreshReply{}, err
	}
	if token.UserID != c.UserID() {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "refresh token has different user", map[string]interface{}{"tokenUser": token.UserID, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.RefreshReply{}, centrifuge.DisconnectInvalidToken
	}
	return centrifuge.RefreshReply{
		ExpireAt: token.ExpireAt,
		Info:     token.Info,
	}, nil
}

// OnRPC ...
func (h *Handler) OnRPC(c *centrifuge.Client, e centrifuge.RPCEvent, rpcProxyHandler proxy.RPCHandlerFunc) (centrifuge.RPCReply, error) {
	if handler, ok := h.rpcExtension[e.Method]; ok {
		return handler(c, e)
	}
	if rpcProxyHandler != nil {
		return rpcProxyHandler(c, e)
	}
	return centrifuge.RPCReply{}, centrifuge.ErrorMethodNotFound
}

// OnSubRefresh ...
func (h *Handler) OnSubRefresh(c *centrifuge.Client, e centrifuge.SubRefreshEvent) (centrifuge.SubRefreshReply, error) {
	token, err := h.tokenVerifier.VerifySubscribeToken(e.Token)
	if err != nil {
		if err == jwtverify.ErrTokenExpired {
			return centrifuge.SubRefreshReply{Expired: true}, nil
		}
		if errors.Is(err, jwtverify.ErrInvalidToken) {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid subscription refresh token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubRefreshReply{}, centrifuge.DisconnectInvalidToken
		}
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error verifying subscription refresh token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubRefreshReply{}, err
	}
	if c.ID() != token.Client || e.Channel != token.Channel {
		return centrifuge.SubRefreshReply{}, centrifuge.DisconnectInvalidToken
	}
	return centrifuge.SubRefreshReply{
		ExpireAt: token.Options.ExpireAt,
		Info:     token.Options.ChannelInfo,
	}, nil
}

// OnSubscribe ...
func (h *Handler) OnSubscribe(c *centrifuge.Client, e centrifuge.SubscribeEvent, subscribeProxyHandler proxy.SubscribeHandlerFunc) (centrifuge.SubscribeReply, error) {
	ruleConfig := h.ruleContainer.Config()

	chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "subscribe channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, centrifuge.ErrorUnknownChannel
	}

	if !chOpts.Anonymous && c.UserID() == "" && !ruleConfig.ClientInsecure {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "anonymous user is not allowed to subscribe on channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied
	}

	isUserLimited := h.ruleContainer.IsUserLimited(e.Channel)

	if isUserLimited && !h.ruleContainer.UserAllowed(e.Channel, c.UserID()) {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "user is not allowed to subscribe on channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied
	}

	var options centrifuge.SubscribeOptions

	isPrivateChannel := h.ruleContainer.IsPrivateChannel(e.Channel)

	if isPrivateChannel {
		if e.Token == "" {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscription token required", map[string]interface{}{"client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied
		}
		token, err := h.tokenVerifier.VerifySubscribeToken(e.Token)
		if err != nil {
			if err == jwtverify.ErrTokenExpired {
				return centrifuge.SubscribeReply{}, centrifuge.ErrorTokenExpired
			}
			if errors.Is(err, jwtverify.ErrInvalidToken) {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid subscription token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
				return centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied
			}
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error verifying subscription token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{}, err
		}
		if c.ID() != token.Client || e.Channel != token.Channel {
			return centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied
		}
		options = token.Options
	} else if chOpts.ProxySubscribe && !h.ruleContainer.IsUserLimited(e.Channel) {
		if subscribeProxyHandler == nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe proxy not enabled", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
			return centrifuge.SubscribeReply{}, centrifuge.ErrorNotAvailable
		}
		return subscribeProxyHandler(c, e, chOpts)
	} else {
		options.Position = chOpts.Position
		options.Recover = chOpts.Recover
		options.Presence = chOpts.Presence
		options.JoinLeave = chOpts.JoinLeave
	}

	if chOpts.Protected && !isPrivateChannel && !isUserLimited {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to subscribe on protected namespace channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied
	}

	return centrifuge.SubscribeReply{
		Options:           options,
		ClientSideRefresh: true,
	}, nil
}

// OnPublish ...
func (h *Handler) OnPublish(c *centrifuge.Client, e centrifuge.PublishEvent, publishProxyHandler proxy.PublishHandlerFunc) (centrifuge.PublishReply, error) {
	ruleConfig := h.ruleContainer.Config()

	chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "publish channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PublishReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "publish to unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PublishReply{}, centrifuge.ErrorUnknownChannel
	}

	if !chOpts.Publish && !ruleConfig.ClientInsecure {
		return centrifuge.PublishReply{}, centrifuge.ErrorPermissionDenied
	}

	if chOpts.SubscribeToPublish {
		if !c.IsSubscribed(e.Channel) {
			return centrifuge.PublishReply{}, centrifuge.ErrorPermissionDenied
		}
	}

	if chOpts.ProxyPublish {
		if publishProxyHandler == nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "publish proxy not enabled", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
			return centrifuge.PublishReply{}, centrifuge.ErrorNotAvailable
		}
		return publishProxyHandler(c, e, chOpts)
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

// OnPresence ...
func (h *Handler) OnPresence(c *centrifuge.Client, e centrifuge.PresenceEvent) (centrifuge.PresenceReply, error) {
	chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "presence channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "presence for unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceReply{}, centrifuge.ErrorUnknownChannel
	}
	if !chOpts.Presence || chOpts.PresenceDisableForClient {
		return centrifuge.PresenceReply{}, centrifuge.ErrorNotAvailable
	}
	if !c.IsSubscribed(e.Channel) {
		return centrifuge.PresenceReply{}, centrifuge.ErrorPermissionDenied
	}
	return centrifuge.PresenceReply{}, nil
}

// OnPresenceStats ...
func (h *Handler) OnPresenceStats(c *centrifuge.Client, e centrifuge.PresenceStatsEvent) (centrifuge.PresenceStatsReply, error) {
	chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "presence stats channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceStatsReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "presence stats for unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceStatsReply{}, centrifuge.ErrorUnknownChannel
	}
	if !chOpts.Presence || chOpts.PresenceDisableForClient {
		return centrifuge.PresenceStatsReply{}, centrifuge.ErrorNotAvailable
	}
	if !c.IsSubscribed(e.Channel) {
		return centrifuge.PresenceStatsReply{}, centrifuge.ErrorPermissionDenied
	}
	return centrifuge.PresenceStatsReply{}, nil
}

// OnHistory ...
func (h *Handler) OnHistory(c *centrifuge.Client, e centrifuge.HistoryEvent) (centrifuge.HistoryReply, error) {
	chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "history channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.HistoryReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "history for unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.HistoryReply{}, centrifuge.ErrorUnknownChannel
	}
	if chOpts.HistorySize <= 0 || chOpts.HistoryTTL <= 0 || chOpts.HistoryDisableForClient {
		return centrifuge.HistoryReply{}, centrifuge.ErrorNotAvailable
	}
	if !c.IsSubscribed(e.Channel) {
		return centrifuge.HistoryReply{}, centrifuge.ErrorPermissionDenied
	}

	if UseUnlimitedHistoryByDefault && e.Filter.Limit == 0 {
		result, err := h.node.History(e.Channel, centrifuge.WithSince(e.Filter.Since), centrifuge.WithLimit(centrifuge.NoLimit))
		if err != nil {
			return centrifuge.HistoryReply{}, err
		}
		return centrifuge.HistoryReply{
			Result: &result,
		}, nil
	}

	return centrifuge.HistoryReply{}, nil
}
