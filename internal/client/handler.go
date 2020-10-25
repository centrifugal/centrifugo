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

// RPCExtensionFunc ...
type RPCExtensionFunc func(c *centrifuge.Client, e centrifuge.RPCEvent) (centrifuge.RPCResult, error)

// Handler ...
type Handler struct {
	node          *centrifuge.Node
	ruleContainer *rule.ChannelRuleContainer
	tokenVerifier jwtverify.Verifier
	proxyConfig   proxy.Config
	rpcExtension  map[string]RPCExtensionFunc
}

// NewHandler ...
func NewHandler(node *centrifuge.Node, ruleContainer *rule.ChannelRuleContainer, tokenVerifier jwtverify.Verifier, proxyConfig proxy.Config) *Handler {
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

func proxyHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: proxy.DefaultMaxIdleConnsPerHost,
		},
		Timeout: timeout,
	}
}

// Setup ...
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

	h.node.OnConnecting(func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectResult, error) {
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

	h.node.OnConnect(func(client *centrifuge.Client) {

		client.OnRefresh(func(event centrifuge.RefreshEvent, cb centrifuge.RefreshCallback) {
			if refreshProxyHandler != nil {
				cb(refreshProxyHandler(client, event))
				return
			}
			cb(h.OnRefresh(client, event))
		})

		if rpcProxyHandler != nil || len(h.rpcExtension) > 0 {
			client.OnRPC(func(event centrifuge.RPCEvent, cb centrifuge.RPCCallback) {
				if handler, ok := h.rpcExtension[event.Method]; ok {
					cb(handler(client, event))
					return
				}
				cb(rpcProxyHandler(client, event))
			})
		}

		client.OnSubscribe(func(event centrifuge.SubscribeEvent, cb centrifuge.SubscribeCallback) {
			cb(h.OnSubscribe(client, event, subscribeProxyHandler))
		})
		client.OnPublish(func(event centrifuge.PublishEvent, cb centrifuge.PublishCallback) {
			cb(h.OnPublish(client, event, publishProxyHandler))
		})
		client.OnSubRefresh(func(event centrifuge.SubRefreshEvent, cb centrifuge.SubRefreshCallback) {
			cb(h.OnSubRefresh(client, event))
		})
		client.OnPresence(func(event centrifuge.PresenceEvent, cb centrifuge.PresenceCallback) {
			cb(h.OnPresence(client, event))
		})
		client.OnPresenceStats(func(event centrifuge.PresenceStatsEvent, cb centrifuge.PresenceStatsCallback) {
			cb(h.OnPresenceStats(client, event))
		})
		client.OnHistory(func(event centrifuge.HistoryEvent, cb centrifuge.HistoryCallback) {
			cb(h.OnHistory(client, event))
		})
	})
}

func toClientErr(err error) *centrifuge.Error {
	if clientErr, ok := err.(*centrifuge.Error); ok {
		return clientErr
	}
	return centrifuge.ErrorInternal
}

// OnClientConnecting ...
func (h *Handler) OnClientConnecting(
	ctx context.Context,
	e centrifuge.ConnectEvent,
	connectProxyHandler centrifuge.ConnectingHandler,
	refreshProxyEnabled bool,
) (centrifuge.ConnectResult, error) {
	var (
		credentials   *centrifuge.Credentials
		subscriptions []centrifuge.Subscription
		data          []byte
	)

	ruleConfig := h.ruleContainer.Config()

	if e.Token != "" {
		token, err := h.tokenVerifier.VerifyConnectToken(e.Token)
		if err != nil {
			if err == jwtverify.ErrTokenExpired {
				return centrifuge.ConnectResult{}, centrifuge.ErrorTokenExpired
			}
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid connection token", map[string]interface{}{"error": err.Error(), "client": e.ClientID}))
			return centrifuge.ConnectResult{}, centrifuge.DisconnectInvalidToken
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
				return centrifuge.ConnectResult{}, err
			}
			if !found {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe unknown channel", map[string]interface{}{"channel": ch}))
				return centrifuge.ConnectResult{}, centrifuge.ErrorUnknownChannel
			}
			subscriptions = append(subscriptions, centrifuge.Subscription{
				Channel:   ch,
				Presence:  chOpts.Presence,
				JoinLeave: chOpts.JoinLeave,
				Recover:   chOpts.HistoryRecover,
			})
		}
	} else if connectProxyHandler != nil {
		connectResult, err := connectProxyHandler(ctx, e)
		if err != nil {
			return centrifuge.ConnectResult{}, err
		}
		credentials = connectResult.Credentials
		subscriptions = connectResult.Subscriptions
		data = connectResult.Data
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
			return centrifuge.ConnectResult{}, err
		}
		if !found {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe unknown personal channel", map[string]interface{}{"channel": personalChannel}))
			return centrifuge.ConnectResult{}, centrifuge.ErrorUnknownChannel
		}
		subscriptions = append(subscriptions, centrifuge.Subscription{
			Channel:   personalChannel,
			Presence:  chOpts.Presence,
			JoinLeave: chOpts.JoinLeave,
			Recover:   chOpts.HistoryRecover,
		})
	}

	return centrifuge.ConnectResult{
		Credentials:       credentials,
		Subscriptions:     subscriptions,
		Data:              data,
		ClientSideRefresh: !refreshProxyEnabled,
	}, nil
}

// OnRefresh ...
func (h *Handler) OnRefresh(c *centrifuge.Client, e centrifuge.RefreshEvent) (centrifuge.RefreshResult, error) {
	token, err := h.tokenVerifier.VerifyConnectToken(e.Token)
	if err != nil {
		if err == jwtverify.ErrTokenExpired {
			return centrifuge.RefreshResult{Expired: true}, nil
		}
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid refresh token", map[string]interface{}{"error": err.Error(), "client": c.ID()}))
		return centrifuge.RefreshResult{}, centrifuge.DisconnectInvalidToken
	}
	return centrifuge.RefreshResult{
		ExpireAt: token.ExpireAt,
		Info:     token.Info,
	}, nil
}

// OnSubRefresh ...
func (h *Handler) OnSubRefresh(c *centrifuge.Client, e centrifuge.SubRefreshEvent) (centrifuge.SubRefreshResult, error) {
	token, err := h.tokenVerifier.VerifySubscribeToken(e.Token)
	if err != nil {
		if err == jwtverify.ErrTokenExpired {
			return centrifuge.SubRefreshResult{Expired: true}, nil
		}
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid subscription refresh token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubRefreshResult{}, centrifuge.DisconnectInvalidToken
	}
	if c.ID() != token.Client || e.Channel != token.Channel {
		return centrifuge.SubRefreshResult{}, centrifuge.DisconnectInvalidToken
	}
	return centrifuge.SubRefreshResult{
		ExpireAt: token.ExpireAt,
		Info:     token.Info,
	}, nil
}

// OnSubscribe ...
func (h *Handler) OnSubscribe(c *centrifuge.Client, e centrifuge.SubscribeEvent, subscribeProxyHandler proxy.SubscribeHandlerFunc) (centrifuge.SubscribeResult, error) {
	ruleConfig := h.ruleContainer.Config()

	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "subscribe channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeResult{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeResult{}, centrifuge.ErrorUnknownChannel
	}

	if chOpts.ServerSide {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to subscribe on server side channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeResult{}, centrifuge.ErrorPermissionDenied
	}

	if !chOpts.Anonymous && c.UserID() == "" && !ruleConfig.ClientInsecure {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "anonymous user is not allowed to subscribe on channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeResult{}, centrifuge.ErrorPermissionDenied
	}

	if !h.ruleContainer.UserAllowed(e.Channel, c.UserID()) {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "user is not allowed to subscribe on channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeResult{}, centrifuge.ErrorPermissionDenied
	}

	var (
		channelInfo []byte
		expireAt    int64
	)

	if h.ruleContainer.IsTokenChannel(e.Channel) {
		if e.Token == "" {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscription token required", map[string]interface{}{"client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeResult{}, centrifuge.ErrorPermissionDenied
		}
		token, err := h.tokenVerifier.VerifySubscribeToken(e.Token)
		if err != nil {
			if err == jwtverify.ErrTokenExpired {
				return centrifuge.SubscribeResult{}, centrifuge.ErrorTokenExpired
			}
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid subscription token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeResult{}, centrifuge.ErrorPermissionDenied
		}
		if c.ID() != token.Client || e.Channel != token.Channel {
			return centrifuge.SubscribeResult{}, centrifuge.ErrorPermissionDenied
		}
		expireAt = token.ExpireAt
		if token.ExpireTokenOnly {
			expireAt = 0
		}
		channelInfo = token.Info
	} else if chOpts.ProxySubscribe && !h.ruleContainer.IsUserLimited(e.Channel) {
		if subscribeProxyHandler == nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe proxy not enabled", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
			return centrifuge.SubscribeResult{}, centrifuge.ErrorNotAvailable
		}
		return subscribeProxyHandler(c, e)
	}

	return centrifuge.SubscribeResult{
		ExpireAt:          expireAt,
		ChannelInfo:       channelInfo,
		ClientSideRefresh: true,
		Presence:          chOpts.Presence,
		JoinLeave:         chOpts.JoinLeave,
		Recover:           chOpts.HistoryRecover,
	}, nil
}

// OnPublish ...
func (h *Handler) OnPublish(c *centrifuge.Client, e centrifuge.PublishEvent, publishProxyHandler proxy.PublishHandlerFunc) (centrifuge.PublishResult, error) {
	ruleConfig := h.ruleContainer.Config()

	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "publish channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PublishResult{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "publish to unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PublishResult{}, centrifuge.ErrorUnknownChannel
	}

	if !chOpts.Publish && !ruleConfig.ClientInsecure {
		return centrifuge.PublishResult{}, centrifuge.ErrorPermissionDenied
	}

	if chOpts.SubscribeToPublish {
		if !c.IsSubscribed(e.Channel) {
			return centrifuge.PublishResult{}, centrifuge.ErrorPermissionDenied
		}
	}

	if chOpts.ProxyPublish {
		if publishProxyHandler == nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "publish proxy not enabled", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
			return centrifuge.PublishResult{}, centrifuge.ErrorNotAvailable
		}
		return publishProxyHandler(c, e, chOpts)
	}

	return h.node.Publish(
		e.Channel, e.Data,
		centrifuge.WithClientInfo(e.Info),
		centrifuge.WithHistory(chOpts.HistorySize, time.Duration(chOpts.HistoryLifetime)*time.Second),
	)
}

// OnPresence ...
func (h *Handler) OnPresence(c *centrifuge.Client, e centrifuge.PresenceEvent) (centrifuge.PresenceResult, error) {
	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "presence channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceResult{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "presence for unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceResult{}, centrifuge.ErrorUnknownChannel
	}
	if !chOpts.Presence || chOpts.PresenceDisableForClient {
		return centrifuge.PresenceResult{}, centrifuge.ErrorNotAvailable
	}
	if !c.IsSubscribed(e.Channel) {
		return centrifuge.PresenceResult{}, centrifuge.ErrorPermissionDenied
	}
	return h.node.Presence(e.Channel)
}

// OnPresenceStats ...
func (h *Handler) OnPresenceStats(c *centrifuge.Client, e centrifuge.PresenceStatsEvent) (centrifuge.PresenceStatsResult, error) {
	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "presence stats channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceStatsResult{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "presence stats for unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceStatsResult{}, centrifuge.ErrorUnknownChannel
	}
	if !chOpts.Presence || chOpts.PresenceDisableForClient {
		return centrifuge.PresenceStatsResult{}, centrifuge.ErrorNotAvailable
	}
	if !c.IsSubscribed(e.Channel) {
		return centrifuge.PresenceStatsResult{}, centrifuge.ErrorPermissionDenied
	}
	return h.node.PresenceStats(e.Channel)
}

// OnHistory ...
func (h *Handler) OnHistory(c *centrifuge.Client, e centrifuge.HistoryEvent) (centrifuge.HistoryResult, error) {
	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "history channel options error", map[string]interface{}{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.HistoryResult{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "history for unknown channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.HistoryResult{}, centrifuge.ErrorUnknownChannel
	}
	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 || chOpts.HistoryDisableForClient {
		return centrifuge.HistoryResult{}, centrifuge.ErrorNotAvailable
	}
	if !c.IsSubscribed(e.Channel) {
		return centrifuge.HistoryResult{}, centrifuge.ErrorPermissionDenied
	}
	return h.node.History(e.Channel, centrifuge.WithNoLimit())
}
