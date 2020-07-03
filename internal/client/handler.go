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
	var connectProxyHandler func(ctx context.Context, t centrifuge.TransportInfo, e centrifuge.ConnectEvent) centrifuge.ConnectReply
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

	h.node.On().ClientConnecting(func(ctx context.Context, info centrifuge.TransportInfo, e centrifuge.ConnectEvent) centrifuge.ConnectReply {
		return h.OnClientConnecting(ctx, info, e, connectProxyHandler, refreshProxyEnabled)
	})

	h.node.On().ClientConnected(func(ctx context.Context, client *centrifuge.Client) {
		client.On().Refresh(func(event centrifuge.RefreshEvent) centrifuge.RefreshReply {
			if refreshProxyEnabled {
				return refreshProxyHandler(ctx, client, event)
			}
			return h.OnRefresh(client, event)
		})
		if rpcProxyEnabled {
			client.On().RPC(func(event centrifuge.RPCEvent) centrifuge.RPCReply {
				return rpcProxyHandler(ctx, client, event)
			})
		}
		client.On().Subscribe(func(event centrifuge.SubscribeEvent) centrifuge.SubscribeReply {
			return h.OnSubscribe(client, event)
		})
		client.On().Publish(func(event centrifuge.PublishEvent) centrifuge.PublishReply {
			return h.OnPublish(client, event)
		})
		client.On().SubRefresh(func(event centrifuge.SubRefreshEvent) centrifuge.SubRefreshReply {
			return h.OnSubRefresh(client, event)
		})
		client.On().Presence(func(event centrifuge.PresenceEvent) centrifuge.PresenceReply {
			return h.OnPresence(client, event)
		})
		client.On().PresenceStats(func(event centrifuge.PresenceStatsEvent) centrifuge.PresenceStatsReply {
			return h.OnPresenceStats(client, event)
		})
		client.On().History(func(event centrifuge.HistoryEvent) centrifuge.HistoryReply {
			return h.OnHistory(client, event)
		})
	})
}

func toClientErr(err error) *centrifuge.Error {
	if clientErr, ok := err.(*centrifuge.Error); ok {
		return clientErr
	}
	return centrifuge.ErrorInternal
}

func (h *Handler) OnClientConnecting(ctx context.Context, info centrifuge.TransportInfo, e centrifuge.ConnectEvent) centrifuge.ConnectReply {
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
				return centrifuge.ConnectReply{Error: centrifuge.ErrorTokenExpired}
			}
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid connection token", map[string]interface{}{"error": err.Error(), "client": e.ClientID}))
			return centrifuge.ConnectReply{Disconnect: centrifuge.DisconnectInvalidToken}
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
		connectReply := connectProxyHandler(ctx, info, e)
		if connectReply.Error != nil {
			return centrifuge.ConnectReply{Error: connectReply.Error}
		}
		if connectReply.Disconnect != nil {
			return centrifuge.ConnectReply{Disconnect: connectReply.Disconnect}
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
	}
}

func (h *Handler) OnRefresh(c *centrifuge.Client, e centrifuge.RefreshEvent) centrifuge.RefreshReply {
	token, err := h.tokenVerifier.VerifyConnectToken(e.Token)
	if err != nil {
		if err == jwtverify.ErrTokenExpired {
			return centrifuge.RefreshReply{Expired: true}
		}
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid refresh token", map[string]interface{}{"error": err.Error(), "client": c.ID()}))
		return centrifuge.RefreshReply{Disconnect: centrifuge.DisconnectInvalidToken}
	}
	return centrifuge.RefreshReply{
		ExpireAt: token.ExpireAt,
		Info:     token.Info,
	}
}

func (h *Handler) OnSubRefresh(c *centrifuge.Client, e centrifuge.SubRefreshEvent) centrifuge.SubRefreshReply {
	token, err := h.tokenVerifier.VerifySubscribeToken(e.Token)
	if err != nil {
		if err == jwtverify.ErrTokenExpired {
			return centrifuge.SubRefreshReply{Expired: true}
		}
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid subscription refresh token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubRefreshReply{Disconnect: centrifuge.DisconnectInvalidToken}
	}
	if c.ID() != token.Client || e.Channel != token.Channel {
		return centrifuge.SubRefreshReply{Disconnect: centrifuge.DisconnectInvalidToken}
	}
	return centrifuge.SubRefreshReply{
		ExpireAt: token.ExpireAt,
		Info:     token.Info,
	}
}

func (h *Handler) OnSubscribe(c *centrifuge.Client, e centrifuge.SubscribeEvent) centrifuge.SubscribeReply {
	ruleConfig := h.ruleContainer.Config()

	chOpts, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe channel options error", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{Error: toClientErr(err)}
	}

	if chOpts.ServerSide {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to subscribe on server side channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{Error: centrifuge.ErrorPermissionDenied}
	}

	if !h.ruleContainer.UserAllowed(e.Channel, c.UserID()) {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "user is not allowed to subscribe on channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{Error: centrifuge.ErrorPermissionDenied}
	}

	if !chOpts.Anonymous && c.UserID() == "" && !ruleConfig.ClientInsecure {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "anonymous user is not allowed to subscribe on channel", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{Error: centrifuge.ErrorPermissionDenied}
	}

	var (
		channelInfo []byte
		expireAt    int64
	)

	if h.ruleContainer.IsTokenChannel(e.Channel) {
		if e.Token == "" {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscription token required", map[string]interface{}{"client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{Error: centrifuge.ErrorPermissionDenied}
		}
		token, err := h.tokenVerifier.VerifySubscribeToken(e.Token)
		if err != nil {
			if err == jwtverify.ErrTokenExpired {
				return centrifuge.SubscribeReply{Error: centrifuge.ErrorTokenExpired}
			}
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid subscription token", map[string]interface{}{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{Error: centrifuge.ErrorPermissionDenied}
		}
		if c.ID() != token.Client || e.Channel != token.Channel {
			return centrifuge.SubscribeReply{Error: centrifuge.ErrorPermissionDenied}
		}
		expireAt = token.ExpireAt
		if token.ExpireTokenOnly {
			expireAt = 0
		}
		channelInfo = token.Info
	}

	return centrifuge.SubscribeReply{
		ExpireAt:          expireAt,
		ChannelInfo:       channelInfo,
		ClientSideRefresh: true,
	}
}

func (h *Handler) OnPublish(c *centrifuge.Client, e centrifuge.PublishEvent) centrifuge.PublishReply {
	ruleConfig := h.ruleContainer.Config()

	chOpts, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "publish channel options error", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PublishReply{Error: toClientErr(err)}
	}

	if chOpts.SubscribeToPublish {
		if _, ok := c.Channels()[e.Channel]; !ok {
			return centrifuge.PublishReply{Error: centrifuge.ErrorPermissionDenied}
		}
	}

	if !chOpts.Publish && !ruleConfig.ClientInsecure {
		return centrifuge.PublishReply{Error: centrifuge.ErrorPermissionDenied}
	}

	return centrifuge.PublishReply{}
}

func (h *Handler) OnPresence(c *centrifuge.Client, e centrifuge.PresenceEvent) centrifuge.PresenceReply {
	chOpts, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "presence channel options error", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceReply{Error: toClientErr(err)}
	}
	if chOpts.PresenceDisableForClient {
		return centrifuge.PresenceReply{Error: centrifuge.ErrorNotAvailable}
	}
	if _, ok := c.Channels()[e.Channel]; !ok {
		return centrifuge.PresenceReply{Error: centrifuge.ErrorPermissionDenied}
	}
	return centrifuge.PresenceReply{}
}

func (h *Handler) OnPresenceStats(c *centrifuge.Client, e centrifuge.PresenceStatsEvent) centrifuge.PresenceStatsReply {
	chOpts, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "presence stats channel options error", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceStatsReply{Error: toClientErr(err)}
	}
	if chOpts.PresenceDisableForClient {
		return centrifuge.PresenceStatsReply{Error: centrifuge.ErrorNotAvailable}
	}
	if _, ok := c.Channels()[e.Channel]; !ok {
		return centrifuge.PresenceStatsReply{Error: centrifuge.ErrorPermissionDenied}
	}
	return centrifuge.PresenceStatsReply{}
}

func (h *Handler) OnHistory(c *centrifuge.Client, e centrifuge.HistoryEvent) centrifuge.HistoryReply {
	chOpts, err := h.ruleContainer.NamespacedChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "history channel options error", map[string]interface{}{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.HistoryReply{Error: toClientErr(err)}
	}
	if chOpts.HistoryDisableForClient {
		return centrifuge.HistoryReply{Error: centrifuge.ErrorNotAvailable}
	}
	if _, ok := c.Channels()[e.Channel]; !ok {
		return centrifuge.HistoryReply{Error: centrifuge.ErrorPermissionDenied}
	}
	return centrifuge.HistoryReply{}
}
