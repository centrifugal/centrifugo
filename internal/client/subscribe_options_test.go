package client

import (
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

// The options only the namespace defines are applied on top of the ones a
// subscription token or a subscribe proxy built, which are kept.
func TestApplyNamespaceSubscribeOptions(t *testing.T) {
	chOpts := configtypes.ChannelOptions{
		HistoryMetaTTL:                  configtypes.Duration(48 * time.Hour),
		MapClientsPresenceChannelPrefix: "clients:",
		MapUsersPresenceChannelPrefix:   "users:",
	}
	chOpts.Map.RemoveClientOnUnsubscribe = true

	options := centrifuge.SubscribeOptions{ExpireAt: 42, EnableRecovery: true}
	applyNamespaceSubscribeOptions(&options, chOpts, "ns:room", centrifuge.SubscriptionTypeMap)
	require.Equal(t, 48*time.Hour, options.HistoryMetaTTL)
	require.True(t, options.MapRemoveClientOnUnsubscribe)
	require.Equal(t, "clients:ns:room", options.MapClientPresenceChannel)
	require.Equal(t, "users:ns:room", options.MapUserPresenceChannel)
	require.Equal(t, int64(42), options.ExpireAt)
	require.True(t, options.EnableRecovery)
	require.Equal(t, centrifuge.SubscriptionTypeMap, options.Type)

	options = centrifuge.SubscribeOptions{}
	applyNamespaceSubscribeOptions(&options, configtypes.ChannelOptions{}, "room", centrifuge.SubscriptionTypeStream)
	require.Equal(t, centrifuge.SubscriptionTypeStream, options.Type)
	require.Empty(t, options.MapClientPresenceChannel)
	require.Empty(t, options.MapUserPresenceChannel)
}
