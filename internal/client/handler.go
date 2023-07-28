package client

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"unicode"

	"github.com/centrifugal/centrifugo/v5/internal/clientstorage"
	"github.com/centrifugal/centrifugo/v5/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v5/internal/proxy"
	"github.com/centrifugal/centrifugo/v5/internal/proxystream"
	"github.com/centrifugal/centrifugo/v5/internal/rule"
	"github.com/centrifugal/centrifugo/v5/internal/subsource"

	"github.com/centrifugal/centrifuge"
)

// RPCExtensionFunc ...
type RPCExtensionFunc func(c Client, e centrifuge.RPCEvent) (centrifuge.RPCReply, error)

// ProxyMap is a structure which contains all configured and already initialized
// proxies which can be used from inside client event handlers.
type ProxyMap struct {
	ConnectProxy      proxy.ConnectProxy
	RefreshProxy      proxy.RefreshProxy
	RpcProxies        map[string]proxy.RPCProxy
	PublishProxies    map[string]proxy.PublishProxy
	SubscribeProxies  map[string]proxy.SubscribeProxy
	SubRefreshProxies map[string]proxy.SubRefreshProxy
	StreamProxyMap    *StreamProxyMap
}

// StreamProxyMap ...
type StreamProxyMap struct {
	ConnectProxy         *proxystream.Proxy
	ConnectBidirectional bool
	SubscribeProxies     map[string]*proxystream.Proxy
}

// Handler ...
type Handler struct {
	node              *centrifuge.Node
	ruleContainer     *rule.Container
	tokenVerifier     *jwtverify.VerifierJWT
	subTokenVerifier  *jwtverify.VerifierJWT
	proxyMap          *ProxyMap
	rpcExtension      map[string]RPCExtensionFunc
	granularProxyMode bool
}

// NewHandler ...
func NewHandler(
	node *centrifuge.Node,
	ruleContainer *rule.Container,
	tokenVerifier *jwtverify.VerifierJWT,
	subTokenVerifier *jwtverify.VerifierJWT,
	proxyMap *ProxyMap,
	granularProxyMode bool,
) *Handler {
	return &Handler{
		node:              node,
		ruleContainer:     ruleContainer,
		tokenVerifier:     tokenVerifier,
		subTokenVerifier:  subTokenVerifier,
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

	var streamHandler *proxystream.Handler
	if h.proxyMap.StreamProxyMap != nil {
		streamHandler = proxystream.NewHandler(proxystream.HandlerConfig{
			ConnectProxy:         h.proxyMap.StreamProxyMap.ConnectProxy,
			ConnectBidirectional: h.proxyMap.StreamProxyMap.ConnectBidirectional,
			SubscribeProxies:     h.proxyMap.StreamProxyMap.SubscribeProxies,
		})
	}

	var connectStreamHandler proxystream.ConnectHandlerFunc
	if streamHandler != nil && h.proxyMap.StreamProxyMap.ConnectProxy != nil {
		connectStreamHandler = streamHandler.HandleConnect(h.node)
	}

	h.node.OnConnecting(func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		reply, err := h.OnClientConnecting(ctx, e, connectProxyHandler, refreshProxyHandler != nil, connectStreamHandler)
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

	var proxySubscribeStreamHandler proxystream.SubscribeHandlerFunc
	if streamHandler != nil && len(h.proxyMap.StreamProxyMap.SubscribeProxies) > 0 {
		proxySubscribeStreamHandler = streamHandler.HandleSubscribe(h.node)
	}

	var subRefreshProxyHandler proxy.SubRefreshHandlerFunc
	if len(h.proxyMap.SubRefreshProxies) > 0 {
		subRefreshProxyHandler = proxy.NewSubRefreshHandler(proxy.SubRefreshHandlerConfig{
			Proxies:           h.proxyMap.SubRefreshProxies,
			GranularProxyMode: h.granularProxyMode,
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
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error calling presence stats", map[string]any{"error": err.Error(), "client": client.ID(), "user": client.UserID()}))
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
					h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error sending disconnect", map[string]any{"error": err.Error(), "client": client.ID(), "user": client.UserID()}))
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

		if h.proxyMap.StreamProxyMap != nil && h.proxyMap.StreamProxyMap.ConnectProxy != nil && h.proxyMap.StreamProxyMap.ConnectBidirectional {
			client.OnMessage(func(e centrifuge.MessageEvent) {
				storage, release := client.AcquireStorage()
				streamSenderKey := "stream_sender"
				sendFunc, ok := storage[streamSenderKey].(proxystream.SendFunc)
				release(storage)
				if ok {
					_ = sendFunc(e.Data)
				} else {
					client.Disconnect(centrifuge.DisconnectNotAvailable)
				}
			})
		}

		client.OnUnsubscribe(func(e centrifuge.UnsubscribeEvent) {
			if h.proxyMap.StreamProxyMap != nil && len(h.proxyMap.StreamProxyMap.SubscribeProxies) > 0 {
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
	connectStreamHandler proxystream.ConnectHandlerFunc,
) (centrifuge.ConnectReply, error) {
	var (
		credentials *centrifuge.Credentials
		data        []byte
		newCtx      context.Context
		clientReady chan *centrifuge.Client
	)

	subscriptions := make(map[string]centrifuge.SubscribeOptions)
	ruleConfig := h.ruleContainer.Config()
	var processClientChannels bool

	storage := map[string]any{}

	if e.Token != "" {
		token, err := h.tokenVerifier.VerifyConnectToken(e.Token)
		if err != nil {
			if err == jwtverify.ErrTokenExpired {
				return centrifuge.ConnectReply{}, centrifuge.ErrorTokenExpired
			}
			if errors.Is(err, jwtverify.ErrInvalidToken) {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid connection token", map[string]any{"error": err.Error(), "client": e.ClientID}))
				return centrifuge.ConnectReply{}, centrifuge.DisconnectInvalidToken
			}
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "internal server error", map[string]any{"error": err.Error(), "client": e.ClientID}))
			return centrifuge.ConnectReply{}, err
		}

		if token.UserID == "" && ruleConfig.DisallowAnonymousConnectionTokens {
			if h.node.LogEnabled(centrifuge.LogLevelDebug) {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "anonymous connection tokens disallowed", map[string]any{"client": e.ClientID}))
			}
			return centrifuge.ConnectReply{}, centrifuge.DisconnectPermissionDenied
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
			storage[clientstorage.KeyMeta] = token.Meta
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
		if connectReply.Storage != nil {
			storage = connectReply.Storage
		}
	} else if connectStreamHandler != nil {
		connectReply, sendFunc, err := connectStreamHandler(ctx, e)
		if err != nil {
			return centrifuge.ConnectReply{}, err
		}
		credentials = connectReply.Credentials
		if connectReply.Subscriptions != nil {
			subscriptions = connectReply.Subscriptions
		}
		if connectReply.ClientReady != nil {
			clientReady = connectReply.ClientReady
		}
		data = connectReply.Data
		newCtx = connectReply.Context
		if connectReply.Storage != nil {
			storage = connectReply.Storage
		}
		if h.proxyMap.StreamProxyMap.ConnectBidirectional {
			if storage == nil {
				storage = make(map[string]any)
			}
			storage["stream_sender"] = sendFunc
		}
	}

	// Proceed with Credentials with empty user ID in case anonymous or insecure options on.
	if credentials == nil && (ruleConfig.AnonymousConnectWithoutToken || ruleConfig.ClientInsecure) {
		credentials = &centrifuge.Credentials{
			UserID: "",
		}
		processClientChannels = true
	}

	// Automatically subscribe on personal server-side channel.
	if credentials != nil && ruleConfig.UserSubscribeToPersonal && credentials.UserID != "" {
		personalChannel := h.ruleContainer.PersonalChannel(credentials.UserID)
		_, _, chOpts, found, err := h.ruleContainer.ChannelOptions(personalChannel)
		if err != nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "subscribe channel options error", map[string]any{"error": err.Error(), "channel": personalChannel}))
			return centrifuge.ConnectReply{}, err
		}
		if !found {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe unknown personal channel", map[string]any{"channel": personalChannel}))
			return centrifuge.ConnectReply{}, centrifuge.ErrorUnknownChannel
		}
		subscriptions[personalChannel] = centrifuge.SubscribeOptions{
			EmitPresence:      chOpts.Presence,
			EmitJoinLeave:     chOpts.JoinLeave,
			PushJoinLeave:     chOpts.ForcePushJoinLeave,
			EnableRecovery:    chOpts.ForceRecovery,
			EnablePositioning: chOpts.ForcePositioning,
			Source:            subsource.UserPersonal,
			HistoryMetaTTL:    time.Duration(chOpts.HistoryMetaTTL),
		}
	}

	if processClientChannels {
		// Try to satisfy client request regarding desired server-side subscriptions.
		// Subscribe only to channels client has permission to, so that it could theoretically
		// just use client-side subscriptions to achieve the same.
		for _, ch := range e.Channels {
			nsName, rest, chOpts, found, err := h.ruleContainer.ChannelOptions(ch)
			if err != nil {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "channel options error", map[string]any{"error": err.Error(), "channel": ch}))
				return centrifuge.ConnectReply{}, err
			}
			if !found {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe unknown channel", map[string]any{"channel": ch}))
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
						Source:            subsource.UniConnect,
						HistoryMetaTTL:    time.Duration(chOpts.HistoryMetaTTL),
					}
				}
			} else {
				if h.node.LogEnabled(centrifuge.LogLevelDebug) {
					h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "ignoring subscription to a channel", map[string]any{"channel": ch, "client": e.ClientID, "user": userID}))
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
		ClientReady:       clientReady,
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

func getStreamPerCallData(c Client) proxystream.PerCallData {
	storage, release := c.AcquireStorage()
	var pcd proxystream.PerCallData
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
		if extra.Meta != nil {
			setStorageMeta(c, extra.Meta)
		}
		return r, RefreshExtra{}, err
	}
	token, err := h.tokenVerifier.VerifyConnectToken(e.Token)
	if err != nil {
		if err == jwtverify.ErrTokenExpired {
			return centrifuge.RefreshReply{Expired: true}, RefreshExtra{}, nil
		}
		if errors.Is(err, jwtverify.ErrInvalidToken) {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid refresh token", map[string]any{"error": err.Error(), "user": c.UserID(), "client": c.ID()}))
			return centrifuge.RefreshReply{}, RefreshExtra{}, centrifuge.DisconnectInvalidToken
		}
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error verifying refresh token", map[string]any{"error": err.Error(), "user": c.UserID(), "client": c.ID()}))
		return centrifuge.RefreshReply{}, RefreshExtra{}, err
	}
	if token.UserID != c.UserID() {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "refresh token user mismatch", map[string]any{"tokenUser": token.UserID, "user": c.UserID(), "client": c.ID()}))
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
		return rpcProxyHandler(c, e, h.ruleContainer, getPerCallData(c))
	}
	return centrifuge.RPCReply{}, centrifuge.ErrorMethodNotFound
}

type SubRefreshExtra struct {
}

// OnSubRefresh ...
func (h *Handler) OnSubRefresh(c Client, subRefreshProxyHandler proxy.SubRefreshHandlerFunc, e centrifuge.SubRefreshEvent) (centrifuge.SubRefreshReply, SubRefreshExtra, error) {
	if e.Token == "" && subRefreshProxyHandler != nil {
		_, _, chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
		if err != nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "sub refresh channel options error", map[string]any{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
			return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, err
		}
		if !found {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "sub refresh unknown channel", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
			return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.ErrorUnknownChannel
		}
		r, _, err := subRefreshProxyHandler(c, e, chOpts, getPerCallData(c))
		return r, SubRefreshExtra{}, err
	}
	tokenVerifier := h.tokenVerifier
	if h.subTokenVerifier != nil {
		tokenVerifier = h.subTokenVerifier
	}
	token, err := tokenVerifier.VerifySubscribeToken(e.Token)
	if err != nil {
		if err == jwtverify.ErrTokenExpired {
			return centrifuge.SubRefreshReply{Expired: true}, SubRefreshExtra{}, nil
		}
		if errors.Is(err, jwtverify.ErrInvalidToken) {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid subscription refresh token", map[string]any{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectInvalidToken
		}
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error verifying subscription refresh token", map[string]any{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, err
	}
	if e.Channel == "" {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "sub refresh empty channel", map[string]any{"tokenChannel": token.Channel, "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectInvalidToken
	}
	if e.Channel != token.Channel {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "sub refresh token channel mismatch", map[string]any{"channel": e.Channel, "tokenChannel": token.Channel, "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectInvalidToken
	}
	if token.UserID != c.UserID() {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "sub refresh token user mismatch", map[string]any{"channel": e.Channel, "tokenUser": token.UserID, "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectInvalidToken
	}
	if token.Client != "" && c.ID() != token.Client {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "token client mismatch", map[string]any{"channel": e.Channel, "client": c.ID(), "user": c.UserID()}))
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

func (h *Handler) validateChannelName(c Client, nsName string, rest string, chOpts rule.ChannelOptions, channel string) error {
	ok, err := h.validChannelName(nsName, rest, chOpts, channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "error checking channel name", map[string]any{"channel": channel, "error": err.Error(), "client": c.ID(), "user": c.UserID()}))
		return centrifuge.ErrorInternal
	}
	if !ok {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid channel name", map[string]any{"channel": channel, "client": c.ID(), "user": c.UserID()}))
		return centrifuge.ErrorBadRequest
	}
	return nil
}

type SubscribeExtra struct {
}

// OnSubscribe ...
func (h *Handler) OnSubscribe(c Client, e centrifuge.SubscribeEvent, subscribeProxyHandler proxy.SubscribeHandlerFunc, subscribeStreamHandlerFunc proxystream.SubscribeHandlerFunc) (centrifuge.SubscribeReply, SubscribeExtra, error) {
	ruleConfig := h.ruleContainer.Config()

	if e.Channel == "" {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe empty channel", map[string]any{"user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorUnknownChannel
	}

	nsName, rest, chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "subscribe channel options error", map[string]any{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe unknown channel", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
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
	options.HistoryMetaTTL = time.Duration(chOpts.HistoryMetaTTL)

	isPrivateChannel := h.ruleContainer.IsPrivateChannel(e.Channel)
	isUserLimitedChannel := chOpts.UserLimitedChannels && h.ruleContainer.IsUserLimited(e.Channel)

	if isPrivateChannel && e.Token == "" {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "attempt to subscribe on private channel without token", map[string]any{"channel": e.Channel, "client": c.ID(), "user": c.UserID()}))
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorPermissionDenied
	}

	if e.Token != "" {
		tokenVerifier := h.tokenVerifier
		if h.subTokenVerifier != nil {
			tokenVerifier = h.subTokenVerifier
		}
		token, err := tokenVerifier.VerifySubscribeToken(e.Token)
		if err != nil {
			if err == jwtverify.ErrTokenExpired {
				return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorTokenExpired
			}
			if errors.Is(err, jwtverify.ErrInvalidToken) {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "invalid subscription token", map[string]any{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
				return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorPermissionDenied
			}
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error verifying subscription token", map[string]any{"error": err.Error(), "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, err
		}
		if e.Channel != token.Channel {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "token channel mismatch", map[string]any{"channel": e.Channel, "tokenChannel": token.Channel, "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.DisconnectInvalidToken
		}
		if token.UserID != c.UserID() {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "token user mismatch", map[string]any{"channel": e.Channel, "tokenUser": token.UserID, "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.DisconnectInvalidToken
		}
		if token.Client != "" && c.ID() != token.Client {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "token client mismatch", map[string]any{"channel": e.Channel, "client": c.ID(), "user": c.UserID()}))
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.DisconnectInvalidToken
		}
		options = token.Options
		allowed = true
		options.Source = subsource.SubscriptionToken
	} else if isUserLimitedChannel && h.ruleContainer.UserAllowed(e.Channel, c.UserID()) {
		allowed = true
		options.Source = subsource.UserLimited
	} else if (chOpts.ProxySubscribe || chOpts.SubscribeProxyName != "") && !isUserLimitedChannel {
		if subscribeProxyHandler == nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe proxy not enabled", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorNotAvailable
		}
		r, _, err := subscribeProxyHandler(c, e, chOpts, getPerCallData(c))
		if chOpts.ProxySubRefresh || chOpts.SubRefreshProxyName != "" {
			r.ClientSideRefresh = false
		}
		return r, SubscribeExtra{}, err
	} else if chOpts.ProxyStreamSubscribe && !isUserLimitedChannel {
		if subscribeStreamHandlerFunc == nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "stream proxy not enabled", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorNotAvailable
		}
		r, publishFunc, cancelFunc, err := subscribeStreamHandlerFunc(c, chOpts.ProxyStreamSubscribeBidirectional, e, chOpts, getStreamPerCallData(c))
		if chOpts.ProxyStreamSubscribeBidirectional {
			storage, release := c.AcquireStorage()
			storage["stream_cancel_"+e.Channel] = cancelFunc
			storage["stream_publisher_"+e.Channel] = publishFunc
			release(storage)
		}
		if chOpts.ProxySubRefresh || chOpts.SubRefreshProxyName != "" {
			r.ClientSideRefresh = false
		}
		return r, SubscribeExtra{}, err
	} else if chOpts.SubscribeForClient && (c.UserID() != "" || chOpts.SubscribeForAnonymous) && !isUserLimitedChannel {
		allowed = true
		options.Source = subsource.ClientAllowed
	} else if ruleConfig.ClientInsecure {
		allowed = true
		options.Source = subsource.ClientInsecure
	}

	if !allowed {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to subscribe without sufficient permission", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorPermissionDenied
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

	return centrifuge.SubscribeReply{
		Options:           options,
		ClientSideRefresh: !chOpts.ProxySubRefresh && chOpts.SubRefreshProxyName == "",
	}, SubscribeExtra{}, nil
}

// OnPublish ...
func (h *Handler) OnPublish(c Client, e centrifuge.PublishEvent, publishProxyHandler proxy.PublishHandlerFunc) (centrifuge.PublishReply, error) {
	ruleConfig := h.ruleContainer.Config()

	nsName, rest, chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "publish channel options error", map[string]any{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PublishReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "publish to unknown channel", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PublishReply{}, centrifuge.ErrorUnknownChannel
	}
	if err = h.validateChannelName(c, nsName, rest, chOpts, e.Channel); err != nil {
		return centrifuge.PublishReply{}, err
	}

	var allowed bool

	if chOpts.ProxyPublish || chOpts.PublishProxyName != "" {
		if publishProxyHandler == nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "publish proxy not enabled", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
			return centrifuge.PublishReply{}, centrifuge.ErrorNotAvailable
		}
		return publishProxyHandler(c, e, chOpts, getPerCallData(c))
	} else if chOpts.ProxyStreamSubscribe {
		if !chOpts.ProxyStreamSubscribeBidirectional {
			return centrifuge.PublishReply{}, centrifuge.ErrorNotAvailable
		}
		storage, release := c.AcquireStorage()
		publisher, ok := storage["stream_publisher_"+e.Channel].(proxystream.PublishFunc)
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
	} else if ruleConfig.ClientInsecure {
		allowed = true
	}

	if !allowed {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to publish without sufficient permission", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PublishReply{}, centrifuge.ErrorPermissionDenied
	}

	result, err := h.node.Publish(
		e.Channel, e.Data,
		centrifuge.WithClientInfo(e.ClientInfo),
		centrifuge.WithHistory(chOpts.HistorySize, time.Duration(chOpts.HistoryTTL), time.Duration(chOpts.HistoryMetaTTL)),
	)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "publish error", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID(), "error": err.Error()}))
	}
	return centrifuge.PublishReply{Result: &result}, err
}

func (h *Handler) hasAccessToPresence(c Client, channel string, chOpts rule.ChannelOptions) bool {
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
func (h *Handler) OnPresence(c Client, e centrifuge.PresenceEvent) (centrifuge.PresenceReply, error) {
	nsName, rest, chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "presence channel options error", map[string]any{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "presence for unknown channel", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
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
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to call presence without sufficient permission", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceReply{}, centrifuge.ErrorPermissionDenied
	}

	return centrifuge.PresenceReply{}, nil
}

// OnPresenceStats ...
func (h *Handler) OnPresenceStats(c Client, e centrifuge.PresenceStatsEvent) (centrifuge.PresenceStatsReply, error) {
	nsName, rest, chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "presence stats channel options error", map[string]any{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceStatsReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "presence stats for unknown channel", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
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
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to call presence stats without sufficient permission", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.PresenceStatsReply{}, centrifuge.ErrorPermissionDenied
	}

	return centrifuge.PresenceStatsReply{}, nil
}

func (h *Handler) hasAccessToHistory(c Client, channel string, chOpts rule.ChannelOptions) bool {
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
func (h *Handler) OnHistory(c Client, e centrifuge.HistoryEvent) (centrifuge.HistoryReply, error) {
	nsName, rest, chOpts, found, err := h.ruleContainer.ChannelOptions(e.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "history channel options error", map[string]any{"error": err.Error(), "channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.HistoryReply{}, err
	}
	if !found {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "history for unknown channel", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
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
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "attempt to call history without sufficient permission", map[string]any{"channel": e.Channel, "user": c.UserID(), "client": c.ID()}))
		return centrifuge.HistoryReply{}, centrifuge.ErrorPermissionDenied
	}

	return centrifuge.HistoryReply{}, nil
}
