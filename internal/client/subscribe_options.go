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
