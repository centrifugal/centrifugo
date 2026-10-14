package client

import (
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
)

// applyNamespaceSubscribeOptions sets the subscribe options which only the
// channel namespace defines. A subscription token or a subscribe proxy builds
// the options of the subscription on its own and does not know them, so they
// are applied whichever way the subscription was authorized. The subscription
// type is the one the client asked for, which OnSubscribe checked against the
// namespace.
func applyNamespaceSubscribeOptions(options *centrifuge.SubscribeOptions, chOpts configtypes.ChannelOptions, channel string, subType centrifuge.SubscriptionType) {
	if subType != centrifuge.SubscriptionTypeStream {
		options.Type = subType
	}
	options.HistoryMetaTTL = chOpts.HistoryMetaTTL.ToDuration()
	options.MapRemoveClientOnUnsubscribe = chOpts.Map.RemoveClientOnUnsubscribe
	if chOpts.MapClientsPresenceChannelPrefix != "" {
		options.MapClientPresenceChannel = chOpts.MapClientsPresenceChannelPrefix + channel
	}
	if chOpts.MapUsersPresenceChannelPrefix != "" {
		options.MapUserPresenceChannel = chOpts.MapUsersPresenceChannelPrefix + channel
	}
}

// applyRequestedPositioningRecovery enables positioning and recovery which a
// subscribing client asked for: when the namespace allows clients to enable
// them, or when the client may read channel history. Positioning and recovery
// forced by the namespace, a subscription token or a subscribe proxy are kept.
func applyRequestedPositioningRecovery(options *centrifuge.SubscribeOptions, e centrifuge.SubscribeEvent, chOpts configtypes.ChannelOptions, historyAllowed func() bool) {
	if e.Positioned && (chOpts.AllowPositioning || historyAllowed()) {
		options.EnablePositioning = true
	}
	if e.Recoverable && (chOpts.AllowRecovery || historyAllowed()) {
		options.EnableRecovery = true
	}
	// In cache recovery mode auto_cache_recover makes Centrifugo deliver the latest
	// publication on subscribe without the client providing an empty "since" itself.
	if chOpts.AutoCacheRecover && options.EnableRecovery && options.RecoveryMode == centrifuge.RecoveryModeCache {
		options.AutoCacheRecover = true
	}
}

// applyRequestedJoinLeave pushes join/leave messages to a subscribing client
// which asked for them, when they are emitted in the channel and the client may
// read channel presence. Whether they are emitted comes from the options built
// for the subscription, so an override of a subscription token or a subscribe
// proxy is respected. Pushing forced for the subscription is kept.
func applyRequestedJoinLeave(options *centrifuge.SubscribeOptions, e centrifuge.SubscribeEvent, presenceAllowed func() bool) {
	if e.JoinLeave && options.EmitJoinLeave && presenceAllowed() {
		options.PushJoinLeave = true
	}
}
