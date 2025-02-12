package client

import (
	"context"
	"encoding/json"
	"errors"
	"unicode"

	"github.com/centrifugal/centrifugo/v6/internal/clientcontext"
	"github.com/centrifugal/centrifugo/v6/internal/clientstorage"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v6/internal/logging"
	"github.com/centrifugal/centrifugo/v6/internal/proxy"
	"github.com/centrifugal/centrifugo/v6/internal/subsource"

	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog/log"
)

// RPCExtensionFunc ...
type RPCExtensionFunc func(c Client, e centrifuge.RPCEvent) (centrifuge.RPCReply, error)

// ProxyMap is a structure which contains all configured and already initialized
// proxies which can be used from inside client event handlers.
type ProxyMap struct {
	ConnectProxy           proxy.ConnectProxy
	RefreshProxy           proxy.RefreshProxy
	RpcProxies             map[string]proxy.RPCProxy
	PublishProxies         map[string]proxy.PublishProxy
	SubscribeProxies       map[string]proxy.SubscribeProxy
	SubRefreshProxies      map[string]proxy.SubRefreshProxy
	SubscribeStreamProxies map[string]*proxy.SubscribeStreamProxy
}

// Handler for client connections.
type Handler struct {
	node             *centrifuge.Node
	cfgContainer     *config.Container
	tokenVerifier    *jwtverify.VerifierJWT
	subTokenVerifier *jwtverify.VerifierJWT
	proxyMap         *ProxyMap
	rpcExtension     map[string]RPCExtensionFunc
}

// NewHandler creates new Handler.
func NewHandler(
	node *centrifuge.Node,
	cfgContainer *config.Container,
	tokenVerifier *jwtverify.VerifierJWT,
	subTokenVerifier *jwtverify.VerifierJWT,
	proxyMap *ProxyMap,
) *Handler {
	return &Handler{
		node:             node,
		cfgContainer:     cfgContainer,
		tokenVerifier:    tokenVerifier,
		subTokenVerifier: subTokenVerifier,
		proxyMap:         proxyMap,
		rpcExtension:     make(map[string]RPCExtensionFunc),
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
		}, h.cfgContainer).Handle()
	}

	var refreshProxyHandler proxy.RefreshHandlerFunc
	if h.proxyMap.RefreshProxy != nil {
		refreshProxyHandler = proxy.NewRefreshHandler(proxy.RefreshHandlerConfig{
			Proxy: h.proxyMap.RefreshProxy,
		}).Handle()
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
			Proxies: h.proxyMap.RpcProxies,
		}).Handle()
	}

	var publishProxyHandler proxy.PublishHandlerFunc
	if len(h.proxyMap.PublishProxies) > 0 {
		publishProxyHandler = proxy.NewPublishHandler(proxy.PublishHandlerConfig{
			Proxies: h.proxyMap.PublishProxies,
		}).Handle(h.node)
	}

	var subscribeProxyHandler proxy.SubscribeHandlerFunc
	if len(h.proxyMap.SubscribeProxies) > 0 {
		subscribeProxyHandler = proxy.NewSubscribeHandler(proxy.SubscribeHandlerConfig{
			Proxies: h.proxyMap.SubscribeProxies,
		}).Handle()
	}

	var proxySubscribeStreamHandler proxy.SubscribeStreamHandlerFunc
	if len(h.proxyMap.SubscribeStreamProxies) > 0 {
		proxySubscribeStreamHandler = proxy.NewSubscribeStreamHandler(proxy.SubscribeStreamHandlerConfig{
			Proxies: h.proxyMap.SubscribeStreamProxies,
		}).Handle()
	}

	var subRefreshProxyHandler proxy.SubRefreshHandlerFunc
	if len(h.proxyMap.SubRefreshProxies) > 0 {
		subRefreshProxyHandler = proxy.NewSubRefreshHandler(proxy.SubRefreshHandlerConfig{
			Proxies: h.proxyMap.SubRefreshProxies,
		}).Handle()
	}

	cfg := h.cfgContainer.Config()
	usePersonalChannel := cfg.Client.SubscribeToUserPersonalChannel.Enabled
	singleConnection := cfg.Client.SubscribeToUserPersonalChannel.SingleConnection
	concurrency := cfg.Client.Concurrency

	h.node.OnConnect(func(client *centrifuge.Client) {
		userID := client.UserID()
		if usePersonalChannel && singleConnection && userID != "" {
			personalChannel := h.cfgContainer.PersonalChannel(userID)
			presenceStats, err := h.node.PresenceStats(personalChannel)
			if err != nil {
				log.Error().Err(err).Str("channel", personalChannel).Str("user", userID).Str("client", client.ID()).Msg("error calling presence stats")
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
					log.Error().Err(err).Str("user", userID).Str("client", client.ID()).Msg("error disconnecting user")
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
				reply, extra, err := h.OnRefresh(client, event, refreshProxyHandler)
				if err != nil {
					cb(reply, err)
					return
				}
				if extra.CheckSubs && tokenChannelsChanged(client, extra.Subs) {
					// Check whether server-side subscriptions changed. If yes – disconnect
					// the client to make its token-based server-side subscriptions actual.
					// Theoretically we could avoid disconnection here using Subscribe/Unsubscribe
					// methods, but disconnection seems the good first step for the scenario which
					// should be pretty rare given the stable nature of server-side subscriptions.
					cb(reply, centrifuge.DisconnectInsufficientState)
					return
				}
				cb(reply, nil)
			})
		})

		if rpcProxyHandler != nil || len(h.rpcExtension) > 0 {
			client.OnRPC(func(event centrifuge.RPCEvent, cb centrifuge.RPCCallback) {
				h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
					reply, err := h.OnRPC(client, event, rpcProxyHandler)
					cb(reply, err)
				})
			})
		}

		client.OnSubscribe(func(event centrifuge.SubscribeEvent, cb centrifuge.SubscribeCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, _, err := h.OnSubscribe(client, event, subscribeProxyHandler, proxySubscribeStreamHandler)
				cb(reply, err)
			})
		})

		client.OnSubRefresh(func(event centrifuge.SubRefreshEvent, cb centrifuge.SubRefreshCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, _, err := h.OnSubRefresh(client, subRefreshProxyHandler, event)
				cb(reply, err)
			})
		})

		client.OnPublish(func(event centrifuge.PublishEvent, cb centrifuge.PublishCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, err := h.OnPublish(client, event, publishProxyHandler)
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

		client.OnUnsubscribe(func(e centrifuge.UnsubscribeEvent) {
			if len(h.proxyMap.SubscribeStreamProxies) > 0 {
				storage, release := client.AcquireStorage()
				streamCancelKey := "stream_cancel_" + e.Channel
				cancelFunc, ok := storage[streamCancelKey].(func())
				if ok {
					cancelFunc()
					delete(storage, streamCancelKey)
					delete(storage, "stream_publisher_"+e.Channel)
				}
				release(storage)
			}
		})
	})
	return nil
}

func tokenChannelsChanged(client *centrifuge.Client, subs map[string]centrifuge.SubscribeOptions) bool {
	// Check new channels.
	for ch := range subs {
		if !client.IsSubscribed(ch) {
			return true
		}
	}
	// Check channels not actual anymore.
	for ch, chCtx := range client.ChannelsWithContext() {
		if chCtx.Source == subsource.ConnectionToken {
			if _, ok := subs[ch]; !ok {
				return true
			}
		}
	}
	return false
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
	cfg := h.cfgContainer.Config()
	var processClientChannels bool

	storage := map[string]any{}

	if e.Token != "" {
		token, err := h.tokenVerifier.VerifyConnectToken(e.Token, cfg.Client.InsecureSkipTokenSignatureVerify)
		if err != nil {
			if errors.Is(err, jwtverify.ErrTokenExpired) {
				return centrifuge.ConnectReply{}, centrifuge.ErrorTokenExpired
			}
			if errors.Is(err, jwtverify.ErrInvalidToken) {
				log.Info().Err(err).Str("client", e.ClientID).Msg("invalid connection token")
				return centrifuge.ConnectReply{}, centrifuge.DisconnectInvalidToken
			}
			log.Error().Err(err).Str("client", e.ClientID).Msg("error verifying connection token")
			return centrifuge.ConnectReply{}, err
		}

		if token.UserID == "" && cfg.Client.DisallowAnonymousConnectionTokens {
			if logging.Enabled(logging.DebugLevel) {
				log.Debug().Str("client", e.ClientID).Msg("anonymous connection tokens disallowed")
			}
			return centrifuge.ConnectReply{}, centrifuge.DisconnectPermissionDenied
		}

		credentials = &centrifuge.Credentials{
			UserID:   token.UserID,
			ExpireAt: token.ExpireAt,
			Info:     token.Info,
		}

		if cfg.Client.Insecure {
			credentials.ExpireAt = 0
		}

		subscriptions = token.Subs

		if token.Meta != nil {
			storage[clientstorage.KeyMeta] = token.Meta
		}

		processClientChannels = true
	} else if connectProxyHandler != nil {
		connectReply, _, err := connectProxyHandler(clientcontext.SetEmulatedHeadersToContext(ctx, e.Headers), e)
		if err != nil {
			return centrifuge.ConnectReply{}, err
		}
		credentials = connectReply.Credentials
		if connectReply.Subscriptions != nil {
			subscriptions = connectReply.Subscriptions
		}
		data = connectReply.Data
		newCtx = connectReply.Context
		if connectReply.Storage != nil {
			storage = connectReply.Storage
		}
	}

	// Proceed with Credentials with empty user ID in case anonymous or insecure options on.
	if credentials == nil && (cfg.Client.AllowAnonymousConnectWithoutToken || cfg.Client.Insecure) {
		credentials = &centrifuge.Credentials{
			UserID: "",
		}
		processClientChannels = true
	}

	// Automatically subscribe on personal server-side channel.
	if credentials != nil && cfg.Client.SubscribeToUserPersonalChannel.Enabled && credentials.UserID != "" {
		personalChannel := h.cfgContainer.PersonalChannel(credentials.UserID)
		_, _, chOpts, found, err := h.cfgContainer.ChannelOptions(personalChannel)
		if err != nil {
			log.Error().Err(err).Str("channel", personalChannel).Msg("error getting personal channel options")
			return centrifuge.ConnectReply{}, err
		}
		if !found {
			log.Info().Str("channel", personalChannel).Msg("subscribe unknown personal channel")
			return centrifuge.ConnectReply{}, centrifuge.ErrorUnknownChannel
		}
		subscriptions[personalChannel] = centrifuge.SubscribeOptions{
			EmitPresence:      chOpts.Presence,
			EmitJoinLeave:     chOpts.JoinLeave,
			PushJoinLeave:     chOpts.ForcePushJoinLeave,
			EnableRecovery:    chOpts.ForceRecovery,
			EnablePositioning: chOpts.ForcePositioning,
			RecoveryMode:      chOpts.GetRecoveryMode(),
			Source:            subsource.UserPersonal,
			HistoryMetaTTL:    chOpts.HistoryMetaTTL.ToDuration(),
			AllowedDeltaTypes: chOpts.AllowedDeltaTypes,
		}
	}

	if processClientChannels {
		// Try to satisfy client request regarding desired server-side subscriptions.
		// Subscribe only to channels client has permission to, so that it could theoretically
		// just use client-side subscriptions to achieve the same.
		for _, ch := range e.Channels {
			_, rest, chOpts, found, err := h.cfgContainer.ChannelOptions(ch)
			if err != nil {
				log.Error().Err(err).Str("channel", ch).Msg("error getting channel options")
				return centrifuge.ConnectReply{}, err
			}
			if !found {
				log.Info().Str("channel", ch).Msg("subscribe unknown channel")
				return centrifuge.ConnectReply{}, centrifuge.DisconnectBadRequest
			}

			var channelOk bool

			isUserLimited := h.cfgContainer.IsUserLimited(ch)
			isPrivate := h.cfgContainer.IsPrivateChannel(ch)

			var userID string
			if credentials != nil {
				userID = credentials.UserID
			}

			validChannelName, err := h.validChannelName(rest, chOpts, ch)
			if err != nil {
				return centrifuge.ConnectReply{}, centrifuge.ErrorInternal
			}

			if !isPrivate && !chOpts.SubscribeProxyEnabled && validChannelName {
				if isUserLimited && chOpts.UserLimitedChannels && (userID != "" && h.cfgContainer.UserAllowed(ch, userID)) {
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
						RecoveryMode:      chOpts.GetRecoveryMode(),
						Source:            subsource.UniConnect,
						HistoryMetaTTL:    chOpts.HistoryMetaTTL.ToDuration(),
						AllowedDeltaTypes: chOpts.AllowedDeltaTypes,
					}
				}
			} else {
				if logging.Enabled(logging.DebugLevel) {
					log.Debug().Str("channel", ch).Str("client", e.ClientID).Str("user", userID).Msg("ignoring subscription to a channel")
				}
			}
		}
	}

	finalReply := centrifuge.ConnectReply{
		Storage:           storage,
		Credentials:       credentials,
		Subscriptions:     subscriptions,
		Data:              data,
		ClientSideRefresh: !refreshProxyEnabled,
	}
	if len(e.Headers) > 0 {
		if newCtx != nil {
			newCtx = clientcontext.SetEmulatedHeadersToContext(newCtx, e.Headers)
		} else {
			newCtx = clientcontext.SetEmulatedHeadersToContext(ctx, e.Headers)
		}
	}
	if newCtx != nil {
		finalReply.Context = newCtx
	}
	return finalReply, nil
}

type RefreshExtra struct {
	Subs      map[string]centrifuge.SubscribeOptions
	CheckSubs bool
}

func getPerCallData(c Client) proxy.PerCallData {
	storage, release := c.AcquireStorage()
	var pcd proxy.PerCallData
	if meta, ok := storage[clientstorage.KeyMeta].(json.RawMessage); ok {
		metaCopy := make([]byte, len(meta))
		copy(metaCopy, meta)
		pcd.Meta = metaCopy
	}
	release(storage)
	return pcd
}

func setStorageMeta(c Client, meta json.RawMessage) {
	storage, release := c.AcquireStorage()
	defer release(storage)
	storage[clientstorage.KeyMeta] = meta
}

// OnRefresh ...
func (h *Handler) OnRefresh(c Client, e centrifuge.RefreshEvent, refreshProxyHandler proxy.RefreshHandlerFunc) (centrifuge.RefreshReply, RefreshExtra, error) {
	if refreshProxyHandler != nil {
		r, extra, err := refreshProxyHandler(c, e, getPerCallData(c))
		if err == nil && extra.Meta != nil {
			setStorageMeta(c, extra.Meta)
		}
		return r, RefreshExtra{}, err
	}
	token, err := h.tokenVerifier.VerifyConnectToken(e.Token, h.cfgContainer.Config().Client.InsecureSkipTokenSignatureVerify)
	if err != nil {
		if errors.Is(err, jwtverify.ErrTokenExpired) {
			return centrifuge.RefreshReply{Expired: true}, RefreshExtra{}, nil
		}
		if errors.Is(err, jwtverify.ErrInvalidToken) {
			log.Info().Err(err).Str("client", c.ID()).Str("user", c.UserID()).Msg("invalid refresh token")
			return centrifuge.RefreshReply{}, RefreshExtra{}, centrifuge.DisconnectInvalidToken
		}
		log.Error().Err(err).Str("client", c.ID()).Str("user", c.UserID()).Msg("error verifying refresh token")
		return centrifuge.RefreshReply{}, RefreshExtra{}, err
	}
	if token.UserID != c.UserID() {
		log.Info().Str("token_user", token.UserID).Str("user", c.UserID()).Str("client", c.ID()).Msg("refresh token user mismatch")
		return centrifuge.RefreshReply{}, RefreshExtra{}, centrifuge.DisconnectInvalidToken
	}
	if token.Meta != nil {
		setStorageMeta(c, token.Meta)
	}
	return centrifuge.RefreshReply{
		ExpireAt: token.ExpireAt,
		Info:     token.Info,
	}, RefreshExtra{Subs: token.Subs, CheckSubs: true}, nil
}

// OnRPC ...
func (h *Handler) OnRPC(c Client, e centrifuge.RPCEvent, rpcProxyHandler proxy.RPCHandlerFunc) (centrifuge.RPCReply, error) {
	if handler, ok := h.rpcExtension[e.Method]; ok {
		return handler(c, e)
	}
	if rpcProxyHandler != nil {
		return rpcProxyHandler(c, e, h.cfgContainer, getPerCallData(c))
	}
	return centrifuge.RPCReply{}, centrifuge.ErrorMethodNotFound
}

type SubRefreshExtra struct {
}

// OnSubRefresh ...
func (h *Handler) OnSubRefresh(c Client, subRefreshProxyHandler proxy.SubRefreshHandlerFunc, e centrifuge.SubRefreshEvent) (centrifuge.SubRefreshReply, SubRefreshExtra, error) {
	if e.Token == "" && subRefreshProxyHandler != nil {
		_, _, chOpts, found, err := h.cfgContainer.ChannelOptions(e.Channel)
		if err != nil {
			log.Error().Err(err).Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("error getting channel options")
			return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, err
		}
		if !found {
			log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("sub refresh unknown channel")
			return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.ErrorUnknownChannel
		}
		r, _, err := subRefreshProxyHandler(c, e, chOpts, getPerCallData(c))
		return r, SubRefreshExtra{}, err
	}
	tokenVerifier := h.tokenVerifier
	if h.subTokenVerifier != nil {
		tokenVerifier = h.subTokenVerifier
	}
	token, err := tokenVerifier.VerifySubscribeToken(e.Token, h.cfgContainer.Config().Client.InsecureSkipTokenSignatureVerify)
	if err != nil {
		if errors.Is(err, jwtverify.ErrTokenExpired) {
			return centrifuge.SubRefreshReply{Expired: true}, SubRefreshExtra{}, nil
		}
		if errors.Is(err, jwtverify.ErrInvalidToken) {
			log.Info().Err(err).Str("client", c.ID()).Str("user", c.UserID()).Str("channel", e.Channel).Msg("invalid subscription refresh token")
			return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectInvalidToken
		}
		log.Error().Err(err).Str("client", c.ID()).Str("user", c.UserID()).Str("channel", e.Channel).Msg("error verifying subscription refresh token")
		return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, err
	}
	if e.Channel == "" {
		log.Info().Str("token_channel", token.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("sub refresh empty channel")
		return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectInvalidToken
	}
	if e.Channel != token.Channel {
		log.Info().Str("token_channel", token.Channel).Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("sub refresh token channel mismatch")
		return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectInvalidToken
	}
	if token.UserID != c.UserID() {
		log.Info().Str("channel", e.Channel).Str("token_user", token.UserID).Str("client", c.ID()).Str("user", c.UserID()).Msg("sub refresh token user mismatch")
		return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectInvalidToken
	}
	if token.Client != "" && c.ID() != token.Client {
		log.Info().Str("channel", e.Channel).Str("token_client", token.Client).Str("client", c.ID()).Str("user", c.UserID()).Msg("sub refresh token client mismatch")
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

func (h *Handler) validChannelName(rest string, chOpts configtypes.ChannelOptions, channel string) (bool, error) {
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

func (h *Handler) validateChannelName(c Client, rest string, chOpts configtypes.ChannelOptions, channel string) error {
	ok, err := h.validChannelName(rest, chOpts, channel)
	if err != nil {
		log.Info().Err(err).Str("channel", channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("error checking channel name")
		return centrifuge.ErrorInternal
	}
	if !ok {
		log.Info().Str("channel", channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("invalid channel name")
		return centrifuge.ErrorBadRequest
	}
	return nil
}

type SubscribeExtra struct {
}

// OnSubscribe ...
func (h *Handler) OnSubscribe(c Client, e centrifuge.SubscribeEvent, subscribeProxyHandler proxy.SubscribeHandlerFunc, subscribeStreamHandlerFunc proxy.SubscribeStreamHandlerFunc) (centrifuge.SubscribeReply, SubscribeExtra, error) {
	cfg := h.cfgContainer.Config()

	if e.Channel == "" {
		log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("subscribe empty channel")
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorUnknownChannel
	}

	_, rest, chOpts, found, err := h.cfgContainer.ChannelOptions(e.Channel)
	if err != nil {
		log.Info().Err(err).Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("error getting channel options")
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, err
	}
	if !found {
		log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("subscribe unknown channel")
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorUnknownChannel
	}
	if err = h.validateChannelName(c, rest, chOpts, e.Channel); err != nil {
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, err
	}

	var allowed bool

	var options centrifuge.SubscribeOptions

	options.EmitPresence = chOpts.Presence
	options.EmitJoinLeave = chOpts.JoinLeave
	options.PushJoinLeave = chOpts.ForcePushJoinLeave
	options.EnablePositioning = chOpts.ForcePositioning
	options.EnableRecovery = chOpts.ForceRecovery
	options.RecoveryMode = chOpts.GetRecoveryMode()
	options.HistoryMetaTTL = chOpts.HistoryMetaTTL.ToDuration()
	options.AllowedDeltaTypes = chOpts.AllowedDeltaTypes

	isPrivateChannel := h.cfgContainer.IsPrivateChannel(e.Channel)
	isUserLimitedChannel := chOpts.UserLimitedChannels && h.cfgContainer.IsUserLimited(e.Channel)

	if isPrivateChannel && e.Token == "" {
		log.Warn().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("attempt to subscribe on private channel without token")
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorPermissionDenied
	}

	if e.Token != "" {
		tokenVerifier := h.tokenVerifier
		if h.subTokenVerifier != nil {
			tokenVerifier = h.subTokenVerifier
		}
		token, err := tokenVerifier.VerifySubscribeToken(e.Token, h.cfgContainer.Config().Client.InsecureSkipTokenSignatureVerify)
		if err != nil {
			if errors.Is(err, jwtverify.ErrTokenExpired) {
				return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorTokenExpired
			}
			if errors.Is(err, jwtverify.ErrInvalidToken) {
				log.Info().Err(err).Str("client", c.ID()).Str("user", c.UserID()).Str("channel", e.Channel).Msg("invalid subscription token")
				return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorPermissionDenied
			}
			log.Error().Err(err).Str("client", c.ID()).Str("user", c.UserID()).Str("channel", e.Channel).Msg("error verifying subscription token")
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, err
		}
		if e.Channel != token.Channel {
			log.Info().Str("channel", e.Channel).Str("token_channel", token.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("token channel mismatch")
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.DisconnectInvalidToken
		}
		if token.UserID != c.UserID() {
			log.Info().Str("channel", e.Channel).Str("token_user", token.UserID).Str("client", c.ID()).Str("user", c.UserID()).Msg("token user mismatch")
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.DisconnectInvalidToken
		}
		if token.Client != "" && c.ID() != token.Client {
			log.Info().Str("channel", e.Channel).Str("token_client", token.Client).Str("client", c.ID()).Str("user", c.UserID()).Msg("token client mismatch")
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.DisconnectInvalidToken
		}
		options = token.Options
		allowed = true
		options.Source = subsource.SubscriptionToken
	} else if isUserLimitedChannel && h.cfgContainer.UserAllowed(e.Channel, c.UserID()) {
		allowed = true
		options.Source = subsource.UserLimited
	} else if (chOpts.SubscribeProxyEnabled) && !isUserLimitedChannel {
		if subscribeProxyHandler == nil {
			log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("subscribe proxy not enabled")
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorNotAvailable
		}
		r, _, err := subscribeProxyHandler(c, e, chOpts, getPerCallData(c))
		if chOpts.SubRefreshProxyEnabled {
			r.ClientSideRefresh = false
		}
		return r, SubscribeExtra{}, err
	} else if (chOpts.SubscribeStreamProxyEnabled) && !isUserLimitedChannel {
		if subscribeStreamHandlerFunc == nil {
			log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("stream proxy not enabled")
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorNotAvailable
		}
		r, publishFunc, cancelFunc, err := subscribeStreamHandlerFunc(c, chOpts.SubscribeStreamBidirectional, e, chOpts, getPerCallData(c))
		if chOpts.SubscribeStreamBidirectional {
			storage, release := c.AcquireStorage()
			storage["stream_cancel_"+e.Channel] = cancelFunc
			storage["stream_publisher_"+e.Channel] = publishFunc
			release(storage)
		}
		if chOpts.SubRefreshProxyEnabled {
			r.ClientSideRefresh = false
		}
		return r, SubscribeExtra{}, err
	} else if chOpts.SubscribeForClient && (c.UserID() != "" || chOpts.SubscribeForAnonymous) && !isUserLimitedChannel {
		allowed = true
		options.Source = subsource.ClientAllowed
	} else if cfg.Client.Insecure {
		allowed = true
		options.Source = subsource.ClientInsecure
	}

	if !allowed {
		log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("attempt to subscribe without sufficient permission")
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorPermissionDenied
	}

	// Note, using forceSubscribed here because all checks are passed but connection is not yet
	// subscribed in Centrifuge registry.
	if e.Positioned && (chOpts.AllowPositioning || h.hasAccessToHistory(c, e.Channel, chOpts, true)) {
		options.EnablePositioning = true
	}
	if e.Recoverable && (chOpts.AllowRecovery || h.hasAccessToHistory(c, e.Channel, chOpts, true)) {
		options.EnableRecovery = true
	}
	if e.JoinLeave && chOpts.JoinLeave && h.hasAccessToPresence(c, e.Channel, chOpts, true) {
		options.PushJoinLeave = true
	}

	return centrifuge.SubscribeReply{
		Options:           options,
		ClientSideRefresh: !chOpts.SubRefreshProxyEnabled,
	}, SubscribeExtra{}, nil
}

// OnPublish ...
func (h *Handler) OnPublish(c Client, e centrifuge.PublishEvent, publishProxyHandler proxy.PublishHandlerFunc) (centrifuge.PublishReply, error) {
	cfg := h.cfgContainer.Config()

	_, rest, chOpts, found, err := h.cfgContainer.ChannelOptions(e.Channel)
	if err != nil {
		log.Error().Err(err).Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("error getting channel options")
		return centrifuge.PublishReply{}, err
	}
	if !found {
		log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("publish unknown channel")
		return centrifuge.PublishReply{}, centrifuge.ErrorUnknownChannel
	}
	if err = h.validateChannelName(c, rest, chOpts, e.Channel); err != nil {
		return centrifuge.PublishReply{}, err
	}

	var allowed bool

	if chOpts.PublishProxyEnabled {
		if publishProxyHandler == nil {
			log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("publish proxy not enabled")
			return centrifuge.PublishReply{}, centrifuge.ErrorNotAvailable
		}
		return publishProxyHandler(c, e, chOpts, getPerCallData(c))
	} else if chOpts.SubscribeStreamProxyEnabled {
		if !chOpts.SubscribeStreamBidirectional {
			return centrifuge.PublishReply{}, centrifuge.ErrorNotAvailable
		}
		storage, release := c.AcquireStorage()
		publisher, ok := storage["stream_publisher_"+e.Channel].(proxy.StreamPublishFunc)
		release(storage)
		if ok {
			err = publisher(e.Data)
			if err != nil {
				return centrifuge.PublishReply{}, err
			}
			return centrifuge.PublishReply{
				Result: &centrifuge.PublishResult{},
			}, nil
		}
		return centrifuge.PublishReply{}, centrifuge.ErrorNotAvailable
	} else if chOpts.PublishForClient && (c.UserID() != "" || chOpts.PublishForAnonymous) {
		allowed = true
	} else if chOpts.PublishForSubscriber && c.IsSubscribed(e.Channel) && (c.UserID() != "" || chOpts.PublishForAnonymous) {
		allowed = true
	} else if cfg.Client.Insecure {
		allowed = true
	}

	if !allowed {
		log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("attempt to publish without sufficient permission")
		return centrifuge.PublishReply{}, centrifuge.ErrorPermissionDenied
	}

	result, err := h.node.Publish(
		e.Channel, e.Data,
		centrifuge.WithClientInfo(e.ClientInfo),
		centrifuge.WithHistory(chOpts.HistorySize, chOpts.HistoryTTL.ToDuration(), chOpts.HistoryMetaTTL.ToDuration()),
		centrifuge.WithDelta(chOpts.DeltaPublish),
	)
	if err != nil {
		log.Error().Err(err).Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("error publishing message")
	}
	return centrifuge.PublishReply{Result: &result}, err
}

func (h *Handler) hasAccessToPresence(c Client, channel string, chOpts configtypes.ChannelOptions, forceSubscribed bool) bool {
	if chOpts.PresenceForClient && (c.UserID() != "" || chOpts.PresenceForAnonymous) {
		return true
	} else if chOpts.PresenceForSubscriber && (forceSubscribed || c.IsSubscribed(channel)) && (c.UserID() != "" || chOpts.PresenceForAnonymous) {
		return true
	} else if h.cfgContainer.Config().Client.Insecure {
		return true
	}
	return false
}

// OnPresence ...
func (h *Handler) OnPresence(c Client, e centrifuge.PresenceEvent) (centrifuge.PresenceReply, error) {
	_, rest, chOpts, found, err := h.cfgContainer.ChannelOptions(e.Channel)
	if err != nil {
		log.Error().Err(err).Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("error getting channel options")
		return centrifuge.PresenceReply{}, err
	}
	if !found {
		log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("presence for unknown channel")
		return centrifuge.PresenceReply{}, centrifuge.ErrorUnknownChannel
	}
	if err = h.validateChannelName(c, rest, chOpts, e.Channel); err != nil {
		return centrifuge.PresenceReply{}, err
	}
	if !chOpts.Presence {
		return centrifuge.PresenceReply{}, centrifuge.ErrorNotAvailable
	}

	allowed := h.hasAccessToPresence(c, e.Channel, chOpts, false)

	if !allowed {
		log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("attempt to call presence without sufficient permission")
		return centrifuge.PresenceReply{}, centrifuge.ErrorPermissionDenied
	}

	return centrifuge.PresenceReply{}, nil
}

// OnPresenceStats ...
func (h *Handler) OnPresenceStats(c Client, e centrifuge.PresenceStatsEvent) (centrifuge.PresenceStatsReply, error) {
	_, rest, chOpts, found, err := h.cfgContainer.ChannelOptions(e.Channel)
	if err != nil {
		log.Error().Err(err).Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("error getting channel options")
		return centrifuge.PresenceStatsReply{}, err
	}
	if !found {
		log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("presence stats for unknown channel")
		return centrifuge.PresenceStatsReply{}, centrifuge.ErrorUnknownChannel
	}
	if err = h.validateChannelName(c, rest, chOpts, e.Channel); err != nil {
		return centrifuge.PresenceStatsReply{}, err
	}
	if !chOpts.Presence {
		return centrifuge.PresenceStatsReply{}, centrifuge.ErrorNotAvailable
	}

	allowed := h.hasAccessToPresence(c, e.Channel, chOpts, false)

	if !allowed {
		log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("attempt to call presence stats without sufficient permission")
		return centrifuge.PresenceStatsReply{}, centrifuge.ErrorPermissionDenied
	}

	return centrifuge.PresenceStatsReply{}, nil
}

func (h *Handler) hasAccessToHistory(c Client, channel string, chOpts configtypes.ChannelOptions, forceSubscribed bool) bool {
	if chOpts.HistoryForClient && (c.UserID() != "" || chOpts.HistoryForAnonymous) {
		return true
	} else if chOpts.HistoryForSubscriber && (forceSubscribed || c.IsSubscribed(channel)) && (c.UserID() != "" || chOpts.HistoryForAnonymous) {
		return true
	} else if h.cfgContainer.Config().Client.Insecure {
		return true
	}
	return false
}

// OnHistory ...
func (h *Handler) OnHistory(c Client, e centrifuge.HistoryEvent) (centrifuge.HistoryReply, error) {
	_, rest, chOpts, found, err := h.cfgContainer.ChannelOptions(e.Channel)
	if err != nil {
		log.Error().Err(err).Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("error getting channel options")
		return centrifuge.HistoryReply{}, err
	}
	if !found {
		log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("history for unknown channel")
		return centrifuge.HistoryReply{}, centrifuge.ErrorUnknownChannel
	}
	if err = h.validateChannelName(c, rest, chOpts, e.Channel); err != nil {
		return centrifuge.HistoryReply{}, err
	}
	if chOpts.HistorySize <= 0 || chOpts.HistoryTTL <= 0 {
		return centrifuge.HistoryReply{}, centrifuge.ErrorNotAvailable
	}

	allowed := h.hasAccessToHistory(c, e.Channel, chOpts, false)

	if !allowed {
		log.Info().Str("channel", e.Channel).Str("client", c.ID()).Str("user", c.UserID()).Msg("attempt to call history without sufficient permission")
		return centrifuge.HistoryReply{}, centrifuge.ErrorPermissionDenied
	}

	return centrifuge.HistoryReply{}, nil
}
