package app

import (
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/confighelpers"
	"github.com/centrifugal/centrifugo/v6/internal/jwtverify"

	"github.com/cristalhq/jwt/v5"
	"github.com/stretchr/testify/require"
)

func hmacToken(t *testing.T, secret string, channel string) string {
	t.Helper()
	signer, err := jwt.NewSignerHS(jwt.HS256, []byte(secret))
	require.NoError(t, err)
	token, err := jwt.NewBuilder(signer).Build(jwtverify.SubscribeTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user"},
		Channel:          channel,
	})
	require.NoError(t, err)
	return token.String()
}

func TestReloadConfigAllOrNothing(t *testing.T) {
	newConfig := func() config.Config {
		cfg := config.DefaultConfig()
		cfg.Client.Token.HMACSecretKey = "new"
		cfg.Client.SubscriptionToken.Enabled = true
		cfg.Client.SubscriptionToken.HMACSecretKey = "new-sub"
		cfg.Client.AllowAnonymousConnectWithoutToken = true
		return cfg
	}

	testCases := []struct {
		name   string
		modify func(cfg *config.Config)
	}{
		{"ok", func(cfg *config.Config) {}},
		{"invalid subscription token public key", func(cfg *config.Config) {
			cfg.Client.SubscriptionToken.RSAPublicKey = "invalid"
		}},
		{"invalid subscription token issuer regex", func(cfg *config.Config) {
			cfg.Client.SubscriptionToken.IssuerRegex = "("
		}},
		{"invalid subscription token JWKS endpoint", func(cfg *config.Config) {
			cfg.Client.SubscriptionToken.JWKSPublicEndpoint = "ftp://example.com/jwks"
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Client.Token.HMACSecretKey = "old"
			cfg.Client.SubscriptionToken.Enabled = true
			cfg.Client.SubscriptionToken.HMACSecretKey = "old-sub"
			cfgContainer, err := config.NewContainer(cfg)
			require.NoError(t, err)
			verifierConfig, err := confighelpers.MakeVerifierConfig(cfg.Client.Token)
			require.NoError(t, err)
			tokenVerifier, err := jwtverify.NewTokenVerifierJWT(verifierConfig, cfgContainer)
			require.NoError(t, err)
			subVerifierConfig, err := confighelpers.MakeVerifierConfig(cfg.Client.SubscriptionToken.Token)
			require.NoError(t, err)
			subTokenVerifier, err := jwtverify.NewTokenVerifierJWT(subVerifierConfig, cfgContainer)
			require.NoError(t, err)

			newCfg := newConfig()
			tc.modify(&newCfg)
			require.NoError(t, newCfg.Validate())
			err = reloadConfig(newCfg, cfgContainer, tokenVerifier, subTokenVerifier)
			reloaded := err == nil
			require.Equal(t, tc.name == "ok", reloaded)

			secret, subSecret := "old", "old-sub"
			if reloaded {
				secret, subSecret = "new", "new-sub"
			}
			_, err = tokenVerifier.VerifyConnectToken(hmacToken(t, secret, ""), false)
			require.NoError(t, err)
			_, err = subTokenVerifier.VerifySubscribeToken(hmacToken(t, subSecret, "test"), false)
			require.NoError(t, err)
			require.Equal(t, reloaded, cfgContainer.Config().Client.AllowAnonymousConnectWithoutToken)
		})
	}
}
