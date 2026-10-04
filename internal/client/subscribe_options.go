package client

import (
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
)

// applyNamespaceSubscribeOptions sets the subscribe options which only the
// channel namespace defines. A subscription token or a subscribe proxy builds
// the options of the subscription on its own and does not know them, so they
// are applied whichever way the subscription was authorized.
func applyNamespaceSubscribeOptions(options *centrifuge.SubscribeOptions, chOpts configtypes.ChannelOptions, channel string) {
	options.HistoryMetaTTL = chOpts.HistoryMetaTTL.ToDuration()
	options.MapRemoveClientOnUnsubscribe = chOpts.Map.RemoveClientOnUnsubscribe
	if chOpts.MapClientsPresenceChannelPrefix != "" {
		options.MapClientPresenceChannel = chOpts.MapClientsPresenceChannelPrefix + channel
	}
	if chOpts.MapUsersPresenceChannelPrefix != "" {
		options.MapUserPresenceChannel = chOpts.MapUsersPresenceChannelPrefix + channel
	}
}
