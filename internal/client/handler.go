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

type Handler struct {
	node          *centrifuge.Node
	ruleContainer *rule.ChannelRuleContainer
	tokenVerifier jwtverify.Verifier
	proxyConfig   proxy.Config
}

func NewHandler(node *centrifuge.Node, ruleContainer *rule.ChannelRuleContainer, tokenVerifier jwtverify.Verifier, proxyConfig proxy.Config) *Handler {
	return &Handler{node: node, ruleContainer: ruleContainer, tokenVerifier: tokenVerifier, proxyConfig: proxyConfig}
}

func proxyHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: proxy.DefaultMaxIdleConnsPerHost,
		},
		Timeout: timeout,
	}
}

func (h *Handler) Setup() {
	var connectProxyHandler func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error)
	if h.proxyConfig.ConnectEndpoint != "" {
		connectProxyHandler = proxy.NewConnectHandler(proxy.ConnectHandlerConfig{
			Proxy: proxy.NewHTTPConnectProxy(
				h.proxyConfig.ConnectEndpoint,
				proxyHTTPClient(h.proxyConfig.ConnectTimeout),
				proxy.WithExtraHeaders(h.proxyConfig.ExtraHTTPHeaders),
			),
		}).Handle(h.node)
	}

	refreshProxyHandler := proxy.NewRefreshHandler(proxy.RefreshHandlerConfig{
		Proxy: proxy.NewHTTPRefreshProxy(
			h.proxyConfig.RefreshEndpoint,
			proxyHTTPClient(h.proxyConfig.RefreshTimeout),
			proxy.WithExtraHeaders(h.proxyConfig.ExtraHTTPHeaders),
		),
	}).Handle(h.node)
	refreshProxyEnabled := h.proxyConfig.RefreshEndpoint != ""

	rpcProxyHandler := proxy.NewRPCHandler(proxy.RPCHandlerConfig{
		Proxy: proxy.NewHTTPRPCProxy(
			h.proxyConfig.RPCEndpoint,
			proxyHTTPClient(h.proxyConfig.RPCTimeout),
			proxy.WithExtraHeaders(h.proxyConfig.ExtraHTTPHeaders),
		),
	}).Handle(h.node)
	rpcProxyEnabled := h.proxyConfig.RPCEndpoint != ""

	h.node.OnConnecting(func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return h.OnClientConnecting(ctx, e, connectProxyHandler, refreshProxyEnabled)
	})

	h.node.OnRefresh(func(client *centrifuge.Client, event centrifuge.RefreshEvent) (centrifuge.RefreshReply, error) {
		if refreshProxyEnabled {
			return refreshProxyHandler(client.Context(), client, event)
		}
		return h.OnRefresh(client, event)
	})
	if rpcProxyEnabled {
		h.node.OnRPC(func(client *centrifuge.Client, event centrifuge.RPCEvent) (centrifuge.RPCReply, error) {
			return rpcProxyHandler(client.Context(), client, event)
		})
	}

	var publishProxyHandler centrifuge.PublishHandler
	if h.proxyConfig.PublishEndpoint != "" {
		publishProxyHandler = proxy.NewPublishHandler(proxy.PublishHandlerConfig{
			Proxy: proxy.NewHTTPPublishProxy(
				h.proxyConfig.PublishEndpoint,
				proxyHTTPClient(h.proxyConfig.PublishTimeout),
				proxy.WithExtraHeaders(h.proxyConfig.ExtraHTTPHeaders),
			),
		}).Handle(h.node)
	}

	var subscribeProxyHandler centrifuge.SubscribeHandler
	if h.proxyConfig.SubscribeEndpoint != "" {
		subscribeProxyHandler = proxy.NewSubscribeHandler(proxy.SubscribeHandlerConfig{
			Proxy: proxy.NewHTTPSubscribeProxy(
				h.proxyConfig.SubscribeEndpoint,
				proxyHTTPClient(h.proxyConfig.SubscribeTimeout),
				proxy.WithExtraHeaders(h.proxyConfig.ExtraHTTPHeaders),
			),
		}).Handle(h.node)
	}

	h.node.OnSubscribe(func(client *centrifuge.Client, event centrifuge.SubscribeEvent) (centrifuge.SubscribeReply, error) {
		return h.OnSubscribe(client, event, subscribeProxyHandler)
	})
	h.node.OnPublish(func(client *centrifuge.Client, event centrifuge.PublishEvent) (centrifuge.PublishReply, error) {
		return h.OnPublish(client, event, publishProxyHandler)
	})
	h.node.OnSubRefresh(func(client *centrifuge.Client, event centrifuge.SubRefreshEvent) (centrifuge.SubRefreshReply, error) {
		return h.OnSubRefresh(client, event)
	})
	h.node.OnPresence(func(client *centrifuge.Client, event centrifuge.PresenceEvent) (centrifuge.PresenceReply, error) {
		return h.OnPresence(client, event)
	})
	h.node.OnPresenceStats(func(client *centrifuge.Client, event centrifuge.PresenceStatsEvent) (centrifuge.PresenceStatsReply, error) {
		return h.OnPresenceStats(client, event)
	})
	h.node.OnHistory(func(client *centrifuge.Client, event centrifuge.HistoryEvent) (centrifuge.HistoryReply, error) {
		return h.OnHistory(client, event)
	})
}

func toClientErr(err error) *centrifuge.Error {
	if clientErr, ok := err.(*centrifuge.Error); ok {
		return clientErr
	}
	return centrifuge.ErrorInternal
}

func (h *Handler) OnClientConnecting(
	ctx context.Context,
	e centrifuge.ConnectEvent,
	connectProxyHandler centrifuge.ConnectingHandler,
	refreshProxyEnabled bool,
) (centrifuge.ConnectReply, error) {
	var (
		credentials *centrifuge.Credentials
		channels    []string
		data        []byte
	)

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

		channels = append(channels, token.Channels...)
	} else if connectProxyHandler != nil {
		connectReply, err := connectProxyHandler(ctx, e)
		if err != nil {
			return centrifuge.ConnectReply{}, err
		}
		credentials = connectReply.Credentials
		channels = connectReply.Channels
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
		channels = append(channels, h.ruleContainer.PersonalChannel(credentials.UserID))
	}

	return centrifuge.ConnectReply{
		Credentials:       credentials,
		Channels:          channels,
		Data:              data,
		ClientSideRefresh: !refreshProxyEnabled,
	}, nil
}

func (h *Handler) OnRefresh(c *centrifuge.Client, e centrifuge.RefreshEvent) (centrifuge.RefreshReply, error) {
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

func (h *Handler) OnSubscribe(c *centrifuge.Client, e centrifuge.SubscribeEvent, subscribeProxyHandler centrifuge.SubscribeHandler) (centrifuge.SubscribeReply, error) {
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
		return subscribeProxyHandler(c, e)
	}

	return centrifuge.SubscribeReply{
		ExpireAt:          expireAt,
		ChannelInfo:       channelInfo,
		ClientSideRefresh: true,
	}, nil
}

func (h *Handler) OnPublish(c *centrifuge.Client, e centrifuge.PublishEvent, publishProxyHandler centrifuge.PublishHandler) (centrifuge.PublishReply, error) {
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

	if chOpts.SubscribeToPublish {
		if _, ok := c.Channels()[e.Channel]; !ok {
			return centrifuge.PublishReply{}, centrifuge.ErrorPermissionDenied
		}
	}

	if !chOpts.Publish && !ruleConfig.ClientInsecure {
		return centrifuge.PublishReply{}, centrifuge.ErrorPermissionDenied
	}

	if chOpts.ProxyPublish {
		if publishProxyHandler == nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "publish proxy not enabled", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
			return centrifuge.PublishReply{}, centrifuge.ErrorNotAvailable
		}
		return publishProxyHandler(c, e)
	}

	return centrifuge.PublishReply{}, nil
}

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
	if chOpts.PresenceDisableForClient {
		return centrifuge.PresenceReply{}, centrifuge.ErrorNotAvailable
	}
	if _, ok := c.Channels()[e.Channel]; !ok {
		return centrifuge.PresenceReply{}, centrifuge.ErrorPermissionDenied
	}
	return centrifuge.PresenceReply{}, nil
}

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
	if chOpts.PresenceDisableForClient {
		return centrifuge.PresenceStatsReply{}, centrifuge.ErrorNotAvailable
	}
	if _, ok := c.Channels()[e.Channel]; !ok {
		return centrifuge.PresenceStatsReply{}, centrifuge.ErrorPermissionDenied
	}
	return centrifuge.PresenceStatsReply{}, nil
}

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
	if chOpts.HistoryDisableForClient {
		return centrifuge.HistoryReply{}, centrifuge.ErrorNotAvailable
	}
	if _, ok := c.Channels()[e.Channel]; !ok {
		return centrifuge.HistoryReply{}, centrifuge.ErrorPermissionDenied
	}
	return centrifuge.HistoryReply{}, nil
}
