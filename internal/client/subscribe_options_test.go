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

func TestApplyRequestedPositioningRecovery(t *testing.T) {
	allowed := func() bool { return true }
	denied := func() bool { return false }

	testCases := []struct {
		name        string
		event       centrifuge.SubscribeEvent
		chOpts      configtypes.ChannelOptions
		options     centrifuge.SubscribeOptions
		history     func() bool
		positioning bool
		recovery    bool
		autoCache   bool
	}{
		{
			name:    "not requested",
			history: allowed,
		},
		{
			name:        "allowed by namespace",
			event:       centrifuge.SubscribeEvent{Positioned: true, Recoverable: true},
			chOpts:      configtypes.ChannelOptions{AllowPositioning: true, AllowRecovery: true},
			history:     denied,
			positioning: true,
			recovery:    true,
		},
		{
			name:        "allowed by history access",
			event:       centrifuge.SubscribeEvent{Positioned: true, Recoverable: true},
			history:     allowed,
			positioning: true,
			recovery:    true,
		},
		{
			name:    "not allowed",
			event:   centrifuge.SubscribeEvent{Positioned: true, Recoverable: true},
			history: denied,
		},
		{
			name:        "forced kept",
			options:     centrifuge.SubscribeOptions{EnablePositioning: true, EnableRecovery: true},
			history:     denied,
			positioning: true,
			recovery:    true,
		},
		{
			name:      "auto cache recover",
			event:     centrifuge.SubscribeEvent{Recoverable: true},
			chOpts:    configtypes.ChannelOptions{AllowRecovery: true, AutoCacheRecover: true},
			options:   centrifuge.SubscribeOptions{RecoveryMode: centrifuge.RecoveryModeCache},
			history:   denied,
			recovery:  true,
			autoCache: true,
		},
		{
			name:     "no auto cache recover in stream recovery mode",
			event:    centrifuge.SubscribeEvent{Recoverable: true},
			chOpts:   configtypes.ChannelOptions{AllowRecovery: true, AutoCacheRecover: true},
			options:  centrifuge.SubscribeOptions{RecoveryMode: centrifuge.RecoveryModeStream},
			history:  denied,
			recovery: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			options := tc.options
			applyRequestedPositioningRecovery(&options, tc.event, tc.chOpts, tc.history)
			require.Equal(t, tc.positioning, options.EnablePositioning)
			require.Equal(t, tc.recovery, options.EnableRecovery)
			require.Equal(t, tc.autoCache, options.AutoCacheRecover)
		})
	}
}

func TestApplyRequestedJoinLeave(t *testing.T) {
	allowed := func() bool { return true }
	denied := func() bool { return false }

	testCases := []struct {
		name     string
		event    centrifuge.SubscribeEvent
		options  centrifuge.SubscribeOptions
		presence func() bool
		push     bool
	}{
		{
			name:     "not requested",
			options:  centrifuge.SubscribeOptions{EmitJoinLeave: true},
			presence: allowed,
		},
		{
			name:     "requested",
			event:    centrifuge.SubscribeEvent{JoinLeave: true},
			options:  centrifuge.SubscribeOptions{EmitJoinLeave: true},
			presence: allowed,
			push:     true,
		},
		{
			name:     "not emitted",
			event:    centrifuge.SubscribeEvent{JoinLeave: true},
			presence: allowed,
		},
		{
			name:     "no presence access",
			event:    centrifuge.SubscribeEvent{JoinLeave: true},
			options:  centrifuge.SubscribeOptions{EmitJoinLeave: true},
			presence: denied,
		},
		{
			name:     "forced kept",
			options:  centrifuge.SubscribeOptions{EmitJoinLeave: true, PushJoinLeave: true},
			presence: denied,
			push:     true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			options := tc.options
			applyRequestedJoinLeave(&options, tc.event, tc.presence)
			require.Equal(t, tc.push, options.PushJoinLeave)
		})
	}
}
