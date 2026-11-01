package jwtverify

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/subsource"

	"github.com/centrifugal/centrifuge"
	"github.com/cristalhq/jwt/v5"
	"github.com/stretchr/testify/require"
)

// subscribeSitesContainer returns a container with namespace "ns" whose
// channel options set every option the subscribe option sites read.
func subscribeSitesContainer(t testing.TB) *config.Container {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
		Name: "ns",
		ChannelOptions: configtypes.ChannelOptions{
			Presence:                    true,
			JoinLeave:                   true,
			ForcePushJoinLeave:          true,
			HistorySize:                 10,
			HistoryTTL:                  configtypes.Duration(time.Minute),
			HistoryMetaTTL:              configtypes.Duration(time.Hour),
			ForceRecovery:               true,
			ForcePositioning:            true,
			ForceRecoveryMode:           "cache",
			AutoCacheRecover:            true,
			RecoveryMaxPublicationLimit: 7,
			AllowedDeltaTypes:           []centrifuge.DeltaType{centrifuge.DeltaTypeFossil},
			AllowTagsFilter:             true,
		},
	}}
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	return cfgContainer
}

func buildHMACToken(t testing.TB, claims any) string {
	t.Helper()
	signer, err := jwt.NewSignerHS(jwt.HS256, []byte("secret"))
	require.NoError(t, err)
	token, err := jwt.NewBuilder(signer).Build(claims)
	require.NoError(t, err)
	return token.String()
}

// TestSubscribeOptionSites pins the subscribe options built from channel
// options at the token sites.
func TestSubscribeOptionSites(t *testing.T) {
	cfgContainer := subscribeSitesContainer(t)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{HMACSecretKey: "secret"}, cfgContainer)
	require.NoError(t, err)

	exp := time.Now().Add(time.Hour).Unix()

	t.Run("connect_token_subs", func(t *testing.T) {
		token := buildHMACToken(t, &ConnectTokenClaims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "42"},
			Subs: map[string]SubscribeOptions{
				"ns:a": {
					Base64Info: "e30=",
					Data:       json.RawMessage(`{"d":1}`),
					Override: &SubscribeOptionOverride{
						Presence:         &BoolValue{Value: false},
						ForcePositioning: &BoolValue{Value: false},
					},
				},
			},
		})
		ct, err := verifier.VerifyConnectToken(token, false)
		require.NoError(t, err)
		require.Equal(t, map[string]centrifuge.SubscribeOptions{
			"ns:a": {
				ChannelInfo:                 []byte("{}"),
				EmitPresence:                false,
				EmitJoinLeave:               true,
				PushJoinLeave:               true,
				EnableRecovery:              true,
				EnablePositioning:           false,
				RecoveryMode:                centrifuge.RecoveryModeCache,
				Data:                        []byte(`{"d":1}`),
				Source:                      subsource.ConnectionToken,
				HistoryMetaTTL:              time.Hour,
				RecoveryMaxPublicationLimit: 7,
				AllowedDeltaTypes:           []centrifuge.DeltaType{centrifuge.DeltaTypeFossil},
				AllowTagsFilter:             true,
			},
		}, ct.Subs)
	})

	t.Run("connect_token_channels", func(t *testing.T) {
		token := buildHMACToken(t, &ConnectTokenClaims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "42"},
			Channels:         []string{"ns:b"},
		})
		ct, err := verifier.VerifyConnectToken(token, false)
		require.NoError(t, err)
		require.Equal(t, map[string]centrifuge.SubscribeOptions{
			"ns:b": {
				EmitPresence:                true,
				EmitJoinLeave:               true,
				PushJoinLeave:               true,
				EnableRecovery:              true,
				EnablePositioning:           true,
				RecoveryMode:                centrifuge.RecoveryModeCache,
				Source:                      subsource.ConnectionToken,
				HistoryMetaTTL:              time.Hour,
				RecoveryMaxPublicationLimit: 7,
				AllowedDeltaTypes:           []centrifuge.DeltaType{centrifuge.DeltaTypeFossil},
				AllowTagsFilter:             true,
			},
		}, ct.Subs)
	})

	t.Run("subscribe_token", func(t *testing.T) {
		token := buildHMACToken(t, &SubscribeTokenClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "42",
				ExpiresAt: jwt.NewNumericDate(time.Unix(exp, 0)),
			},
			Channel: "ns:c",
			SubscribeOptions: SubscribeOptions{
				Base64Info: "e30=",
				Data:       json.RawMessage(`{"d":2}`),
				Override: &SubscribeOptionOverride{
					ForceRecovery: &BoolValue{Value: false},
				},
			},
		})
		st, err := verifier.VerifySubscribeToken(token, false)
		require.NoError(t, err)
		require.Equal(t, centrifuge.SubscribeOptions{
			ExpireAt:          exp,
			ChannelInfo:       []byte("{}"),
			EmitPresence:      true,
			EmitJoinLeave:     true,
			PushJoinLeave:     true,
			EnableRecovery:    false,
			EnablePositioning: true,
			RecoveryMode:      centrifuge.RecoveryModeCache,
			AllowedDeltaTypes: []centrifuge.DeltaType{centrifuge.DeltaTypeFossil},
			AllowTagsFilter:   true,
			Data:              []byte(`{"d":2}`),
		}, st.Options)
	})
}

func BenchmarkSubscribeOptionSites(b *testing.B) {
	cfgContainer := subscribeSitesContainer(b)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{HMACSecretKey: "secret"}, cfgContainer)
	require.NoError(b, err)

	connectTokenSubs := buildHMACToken(b, &ConnectTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "42"},
		Subs:             map[string]SubscribeOptions{"ns:a": {Base64Info: "e30="}},
	})
	connectTokenChannels := buildHMACToken(b, &ConnectTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "42"},
		Channels:         []string{"ns:b"},
	})
	subscribeToken := buildHMACToken(b, &SubscribeTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "42"},
		Channel:          "ns:c",
	})

	b.Run("connect_token_subs", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := verifier.VerifyConnectToken(connectTokenSubs, false); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("connect_token_channels", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := verifier.VerifyConnectToken(connectTokenChannels, false); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("subscribe_token", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := verifier.VerifySubscribeToken(subscribeToken, false); err != nil {
				b.Fatal(err)
			}
		}
	})
}
