package client

import (
	"context"
	"net/http"
	"time"

	"github.com/centrifugal/centrifugo/internal/jwtverify"
	"github.com/centrifugal/centrifugo/internal/proxy"
	"github.com/centrifugal/centrifugo/internal/rule"

	"github.com/centrifugal/centrifuge"
)

func proxyHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: proxy.DefaultMaxIdleConnsPerHost,
		},
		Timeout: timeout,
	}
}

// RPCExtensionFunc ...
type RPCExtensionFunc func(c *centrifuge.Client, e centrifuge.RPCEvent) (centrifuge.RPCReply, error)

// Handler ...
type Handler struct {
	node          *centrifuge.Node
	ruleContainer *rule.ChannelRuleContainer
	tokenVerifier jwtverify.Verifier
	proxyConfig   proxy.Config
	rpcExtension  map[string]RPCExtensionFunc
}

// NewHandler ...
func NewHandler(
	node *centrifuge.Node,
	ruleContainer *rule.ChannelRuleContainer,
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
func (h Handler) SetRPCExtension(method string, handler RPCExtensionFunc) {
	h.rpcExtension[method] = handler
}

// Setup event handlers.
func (h *Handler) Setup() {
	var connectProxyHandler centrifuge.ConnectingHandler
	if h.proxyConfig.ConnectEndpoint != "" {
		connectProxyHandler = proxy.NewConnectHandler(proxy.ConnectHandlerConfig{
			Proxy: proxy.NewHTTPConnectProxy(
				h.proxyConfig.ConnectEndpoint,
				proxyHTTPClient(h.proxyConfig.ConnectTimeout),
				proxy.WithExtraHeaders(h.proxyConfig.ExtraHTTPHeaders),
			),
		}, h.ruleContainer).Handle(h.node)
	}

	var refreshProxyHandler proxy.RefreshHandlerFunc
	if h.proxyConfig.RefreshEndpoint != "" {
		refreshProxyHandler = proxy.NewRefreshHandler(proxy.RefreshHandlerConfig{
			Proxy: proxy.NewHTTPRefreshProxy(
				h.proxyConfig.RefreshEndpoint,
				proxyHTTPClient(h.proxyConfig.RefreshTimeout),
				proxy.WithExtraHeaders(h.proxyConfig.ExtraHTTPHeaders),
			),
		}).Handle(h.node)
	}

	h.node.OnConnecting(func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return h.OnClientConnecting(ctx, e, connectProxyHandler, refreshProxyHandler != nil)
	})

	var rpcProxyHandler proxy.RPCHandlerFunc
	if h.proxyConfig.RPCEndpoint != "" {
		rpcProxyHandler = proxy.NewRPCHandler(proxy.RPCHandlerConfig{
			Proxy: proxy.NewHTTPRPCProxy(
				h.proxyConfig.RPCEndpoint,
				proxyHTTPClient(h.proxyConfig.RPCTimeout),
				proxy.WithExtraHeaders(h.proxyConfig.ExtraHTTPHeaders),
			),
		}).Handle(h.node)
	}

	var publishProxyHandler proxy.PublishHandlerFunc
	if h.proxyConfig.PublishEndpoint != "" {
		publishProxyHandler = proxy.NewPublishHandler(proxy.PublishHandlerConfig{
			Proxy: proxy.NewHTTPPublishProxy(
				h.proxyConfig.PublishEndpoint,
				proxyHTTPClient(h.proxyConfig.PublishTimeout),
				proxy.WithExtraHeaders(h.proxyConfig.ExtraHTTPHeaders),
			),
		}).Handle(h.node)
	}

	var subscribeProxyHandler proxy.SubscribeHandlerFunc
	if h.proxyConfig.SubscribeEndpoint != "" {
		subscribeProxyHandler = proxy.NewSubscribeHandler(proxy.SubscribeHandlerConfig{
			Proxy: proxy.NewHTTPSubscribeProxy(
				h.proxyConfig.SubscribeEndpoint,
				proxyHTTPClient(h.proxyConfig.SubscribeTimeout),
				proxy.WithExtraHeaders(h.proxyConfig.ExtraHTTPHeaders),
			),
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
					centrifuge.WithClientWhitelist([]string{client.ID()}),
				)
				if err != nil {
					h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error sending disconnect", map[string]interface{}{"error": err.Error(), "client": client.ID(), "user": client.UserID()}))
					client.Disconnect(centrifuge.DisconnectServerError)
					return
				}
			}
		}

		var semaphore chan struct{}
		if concurrency > 0 {
			semaphore = make(chan struct{}, concurrency)
		}

		client.OnRefresh(func(event centrifuge.RefreshEvent, cb centrifuge.RefreshCallback) {
			if concurrency > 0 {
				semaphore <- struct{}{}
				go func() {
					defer func() { <-semaphore }()
					cb(h.OnRefresh(client, event, refreshProxyHandler))
				}()
			} else {
				cb(h.OnRefresh(client, event, refreshProxyHandler))
			}
		})

		if rpcProxyHandler != nil || len(h.rpcExtension) > 0 {
			client.OnRPC(func(event centrifuge.RPCEvent, cb centrifuge.RPCCallback) {
				if concurrency > 0 {
					semaphore <- struct{}{}
					go func() {
						defer func() { <-semaphore }()
						cb(h.OnRPC(client, event, rpcProxyHandler))
					}()
				} else {
					cb(h.OnRPC(client, event, rpcProxyHandler))
				}
			})
		}

		client.OnSubscribe(func(event centrifuge.SubscribeEvent, cb centrifuge.SubscribeCallback) {
			if concurrency > 0 {
				semaphore <- struct{}{}
				go func() {
					defer func() { <-semaphore }()
					cb(h.OnSubscribe(client, event, subscribeProxyHandler))
				}()
			} else {
				cb(h.OnSubscribe(client, event, subscribeProxyHandler))
			}
		})

		client.OnSubRefresh(func(event centrifuge.SubRefreshEvent, cb centrifuge.SubRefreshCallback) {
			cb(h.OnSubRefresh(client, event))
		})

		client.OnPublish(func(event centrifuge.PublishEvent, cb centrifuge.PublishCallback) {
			if concurrency > 0 {
				semaphore <- struct{}{}
				go func() {
					defer func() { <-semaphore }()
					cb(h.OnPublish(client, event, publishProxyHandler))
				}()
			} else {
				cb(h.OnPublish(client, event, publishProxyHandler))
			}
		})

		client.OnPresence(func(event centrifuge.PresenceEvent, cb centrifuge.PresenceCallback) {
			if concurrency > 0 {
				semaphore <- struct{}{}
				go func() {
					defer func() { <-semaphore }()
					cb(h.OnPresence(client, event))
				}()
			} else {
				cb(h.OnPresence(client, event))
			}
		})

		client.OnPresenceStats(func(event centrifuge.PresenceStatsEvent, cb centrifuge.PresenceStatsCallback) {
			if concurrency > 0 {
				semaphore <- struct{}{}
				go func() {
					defer func() { <-semaphore }()
					cb(h.OnPresenceStats(client, event))
				}()
			} else {
				cb(h.OnPresenceStats(client, event))
			}
		})

		client.OnHistory(func(event centrifuge.HistoryEvent, cb centrifuge.HistoryCallback) {
			if concurrency > 0 {
				semaphore <- struct{}{}
				go func() {
					defer func() { <-semaphore }()
					cb(h.OnHistory(client, event))
				}()
			} else {
				cb(h.OnHistory(client, event))
			}
		})
	})
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
	)

	subscriptions := make(map[string]centrifuge.SubscribeOptions)

	ruleConfig := h.ruleContainer.Config()

	if e.Token != "" {
		token, err := h.tokenVerifier.VerifyConnectToken(e.Token)
		if err != nil {
			if err == jwtverify.ErrTokenExpired {
				return centrifuge.ConnectReply{}, centrifuge.ErrorTokenExpired
			}
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid connection token", map[string]interface{}{"error": err.Error(), "client": e.ClientID}))
			return centrifuge.ConnectReply{}, centrifuge.DisconnectInvalidToken
		}

		credentials = &centrifuge.Credentials{
			UserID:   token.UserID,
			ExpireAt: token.ExpireAt,
			Info:     token.Info,
		}

		if ruleConfig.ClientInsecure {
			credentials.ExpireAt = 0
		}

		for _, ch := range token.Channels {
			chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(ch)
			if err != nil {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "subscribe channel options error", map[string]interface{}{"error": err.Error(), "channel": ch}))
				return centrifuge.ConnectReply{}, err
			}
			if !found {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe unknown channel", map[string]interface{}{"channel": ch}))
				return centrifuge.ConnectReply{}, centrifuge.ErrorUnknownChannel
			}
			subscriptions[ch] = centrifuge.SubscribeOptions{
				Presence:  chOpts.Presence,
				JoinLeave: chOpts.JoinLeave,
				Recover:   chOpts.HistoryRecover,
			}
		}
	} else if connectProxyHandler != nil {
		connectReply, err := connectProxyHandler(ctx, e)
		if err != nil {
			return centrifuge.ConnectReply{}, err
		}
		credentials = connectReply.Credentials
		subscriptions = connectReply.Subscriptions
		data = connectReply.Data
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
		chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(personalChannel)
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
			Recover:   chOpts.HistoryRecover,
		}
	}

	return centrifuge.ConnectReply{
		Credentials:       credentials,
		Subscriptions:     subscriptions,
		Data:              data,
		ClientSideRefresh: !refreshProxyEnabled,
	}, nil
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
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid refresh token", map[string]interface{}{"error": err.Error(), "client": c.ID()}))
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
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid subscription refresh token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubRefreshReply{}, centrifuge.DisconnectInvalidToken
	}
	if c.ID() != token.Client || e.Channel != token.Channel {
		return centrifuge.SubRefreshReply{}, centrifuge.DisconnectInvalidToken
	}
	return centrifuge.SubRefreshReply{
		ExpireAt: token.ExpireAt,
		Info:     token.Info,
	}, nil
}

// OnSubscribe ...
func (h *Handler) OnSubscribe(c *centrifuge.Client, e centrifuge.SubscribeEvent, subscribeProxyHandler proxy.SubscribeHandlerFunc) (centrifuge.SubscribeReply, error) {
	ruleConfig := h.ruleContainer.Config()

	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "subscribe channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, centrifuge.ErrorUnknownChannel
	}

	if chOpts.ServerSide {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to subscribe on server side channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied
	}

	if !chOpts.Anonymous && c.UserID() == "" && !ruleConfig.ClientInsecure {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "anonymous user is not allowed to subscribe on channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied
	}

	if !h.ruleContainer.UserAllowed(e.Channel, c.UserID()) {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "user is not allowed to subscribe on channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied
	}

	var (
		channelInfo []byte
		expireAt    int64
	)

	if h.ruleContainer.IsTokenChannel(e.Channel) {
		if e.Token == "" {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscription token required", map[string]interface{}{"client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied
		}
		token, err := h.tokenVerifier.VerifySubscribeToken(e.Token)
		if err != nil {
			if err == jwtverify.ErrTokenExpired {
				return centrifuge.SubscribeReply{}, centrifuge.ErrorTokenExpired
			}
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid subscription token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied
		}
		if c.ID() != token.Client || e.Channel != token.Channel {
			return centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied
		}
		expireAt = token.ExpireAt
		if token.ExpireTokenOnly {
			expireAt = 0
		}
		channelInfo = token.Info
	} else if chOpts.ProxySubscribe && !h.ruleContainer.IsUserLimited(e.Channel) {
		if subscribeProxyHandler == nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe proxy not enabled", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
			return centrifuge.SubscribeReply{}, centrifuge.ErrorNotAvailable
		}
		return subscribeProxyHandler(c, e, chOpts)
	}

	return centrifuge.SubscribeReply{
		Options: centrifuge.SubscribeOptions{
			ExpireAt:    expireAt,
			ChannelInfo: channelInfo,
			Presence:    chOpts.Presence,
			JoinLeave:   chOpts.JoinLeave,
			Recover:     chOpts.HistoryRecover,
		},
		ClientSideRefresh: true,
	}, nil
}

// OnPublish ...
func (h *Handler) OnPublish(c *centrifuge.Client, e centrifuge.PublishEvent, publishProxyHandler proxy.PublishHandlerFunc) (centrifuge.PublishReply, error) {
	ruleConfig := h.ruleContainer.Config()

	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
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
		centrifuge.WithHistory(chOpts.HistorySize, time.Duration(chOpts.HistoryLifetime)*time.Second),
	)
	return centrifuge.PublishReply{Result: &result}, err
}

// OnPresence ...
func (h *Handler) OnPresence(c *centrifuge.Client, e centrifuge.PresenceEvent) (centrifuge.PresenceReply, error) {
	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
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
	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
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
	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "history channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.HistoryReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "history for unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.HistoryReply{}, centrifuge.ErrorUnknownChannel
	}
	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 || chOpts.HistoryDisableForClient {
		return centrifuge.HistoryReply{}, centrifuge.ErrorNotAvailable
	}
	if !c.IsSubscribed(e.Channel) {
		return centrifuge.HistoryReply{}, centrifuge.ErrorPermissionDenied
	}
	return centrifuge.HistoryReply{}, nil
}
