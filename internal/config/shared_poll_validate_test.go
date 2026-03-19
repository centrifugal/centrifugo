package config

import (
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/stretchr/testify/require"
)

func sharedPollDefaultProxy() configtypes.Proxy {
	return configtypes.Proxy{
		Endpoint: "http://localhost:3001/refresh",
		Timeout:  configtypes.Duration(5 * time.Second),
	}
}

func TestSharedPollConfig_Validation_MissingSecret(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Channel.Proxy.SharedPollRefresh = sharedPollDefaultProxy()
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		{
			Name: "poll",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionType: "shared_poll",
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "hmac_secret_key")
}

func TestSharedPollConfig_Validation_WithSecret(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SharedPoll.HMACSecretKey = "my-secret"
	cfg.Channel.Proxy.SharedPollRefresh = sharedPollDefaultProxy()
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		{
			Name: "poll",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionType: "shared_poll",
			},
		},
	}
	err := cfg.Validate()
	require.NoError(t, err)
}

func TestSharedPollConfig_Validation_InvalidProxyName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SharedPoll.HMACSecretKey = "my-secret"
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		{
			Name: "poll",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionType: "shared_poll",
				SharedPoll: configtypes.SharedPollConfig{
					ProxyName: "nonexistent_proxy",
				},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "shared poll proxy")
	require.Contains(t, err.Error(), "nonexistent_proxy")
}

func TestSharedPollConfig_Validation_ValidProxyName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SharedPoll.HMACSecretKey = "my-secret"
	cfg.Proxies = []configtypes.NamedProxy{
		{
			Name: "poll_backend",
			Proxy: configtypes.Proxy{
				Endpoint: "http://localhost:3001/refresh",
				Timeout:  configtypes.Duration(5 * time.Second),
			},
		},
	}
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		{
			Name: "poll",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionType: "shared_poll",
				SharedPoll: configtypes.SharedPollConfig{
					ProxyName: "poll_backend",
				},
			},
		},
	}
	err := cfg.Validate()
	require.NoError(t, err)
}

func TestSharedPollConfig_SubscriptionTypeValid(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SharedPoll.HMACSecretKey = "secret"
	cfg.Channel.Proxy.SharedPollRefresh = sharedPollDefaultProxy()
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		{
			Name: "poll",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionType: "shared_poll",
			},
		},
	}
	err := cfg.Validate()
	require.NoError(t, err)
}

func TestSharedPollConfig_UnknownSubscriptionType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Channel.WithoutNamespace.SubscriptionType = "unknown_type"
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown subscription_type")
}

func TestSharedPollConfig_WithoutNamespaceValidation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SharedPoll.HMACSecretKey = "secret"
	cfg.Channel.Proxy.SharedPollRefresh = sharedPollDefaultProxy()
	cfg.Channel.WithoutNamespace.SubscriptionType = "shared_poll"
	err := cfg.Validate()
	require.NoError(t, err)
}

func TestSharedPollConfig_WithoutNamespaceMissingSecret(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Channel.Proxy.SharedPollRefresh = sharedPollDefaultProxy()
	cfg.Channel.WithoutNamespace.SubscriptionType = "shared_poll"
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "hmac_secret_key")
}
