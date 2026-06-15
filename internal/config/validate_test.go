package config

import (
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/stretchr/testify/require"
)

func TestValidatePublicationDataFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{
			name:    "empty string is valid",
			format:  "",
			wantErr: false,
		},
		{
			name:    "json is valid",
			format:  configtypes.PublicationDataFormatJSON,
			wantErr: false,
		},
		{
			name:    "json_object is valid",
			format:  configtypes.PublicationDataFormatJSONObject,
			wantErr: false,
		},
		{
			name:    "binary is valid",
			format:  configtypes.PublicationDataFormatBinary,
			wantErr: false,
		},
		{
			name:    "unknown format xml is invalid",
			format:  "xml",
			wantErr: true,
		},
		{
			name:    "unknown format text is invalid",
			format:  "text",
			wantErr: true,
		},
		{
			name:    "random string is invalid",
			format:  "foobar",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Channel.WithoutNamespace.PublicationDataFormat = tt.format
			err := cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "unknown publication_data_format")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidatePublicationDataFormatInNamespace(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{
			name:    "json in namespace",
			format:  "json",
			wantErr: false,
		},
		{
			name:    "json_object in namespace",
			format:  "json_object",
			wantErr: false,
		},
		{
			name:    "binary in namespace",
			format:  "binary",
			wantErr: false,
		},
		{
			name:    "invalid format in namespace",
			format:  "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
				{
					Name: "test",
					ChannelOptions: configtypes.ChannelOptions{
						PublicationDataFormat: tt.format,
					},
				},
			}
			err := cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "unknown publication_data_format")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateGlobalPublicationDataFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{
			name:    "empty global format is valid",
			format:  "",
			wantErr: false,
		},
		{
			name:    "json global format is valid",
			format:  "json",
			wantErr: false,
		},
		{
			name:    "binary global format is valid",
			format:  "binary",
			wantErr: false,
		},
		{
			name:    "invalid global format",
			format:  "xml",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Channel.PublicationDataFormat = tt.format
			err := cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "unknown channel.publication_data_format")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// mapDefaultConfig returns a Config suitable as a base for map namespace validation tests.
func mapDefaultConfig() Config {
	cfg := DefaultConfig()
	return cfg
}

func mapNamespace(name, mode string) configtypes.ChannelNamespace {
	return configtypes.ChannelNamespace{
		Name: name,
		ChannelOptions: configtypes.ChannelOptions{
			SubscriptionType: "map",
			Map:              configtypes.MapConfig{Mode: mode},
		},
	}
}

func TestValidateMapNamespace_Ephemeral(t *testing.T) {
	t.Run("valid_minimal", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ee", "ephemeral")
		ns.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("missing_key_ttl", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ee", "ephemeral")
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map.key_ttl is required")
	})

	t.Run("stream_size_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ee", "ephemeral")
		ns.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		ns.Map.StreamSize = 100
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map.stream_size must be 0")
	})

	t.Run("stream_ttl_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ee", "ephemeral")
		ns.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		ns.Map.StreamTTL = configtypes.Duration(time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map.stream_ttl must be 0")
	})

	t.Run("meta_ttl_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ee", "ephemeral")
		ns.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		ns.Map.MetaTTL = configtypes.Duration(10 * time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map.meta_ttl must be 0")
	})
}

func TestValidateAutoCacheRecover(t *testing.T) {
	t.Run("valid_with_cache_recovery", func(t *testing.T) {
		cfg := DefaultConfig()
		opts := &cfg.Channel.WithoutNamespace
		opts.HistorySize = 1
		opts.HistoryTTL = configtypes.Duration(time.Hour)
		opts.ForceRecovery = true
		opts.ForceRecoveryMode = "cache"
		opts.AutoCacheRecover = true
		require.NoError(t, cfg.Validate())
	})

	t.Run("requires_force_recovery", func(t *testing.T) {
		cfg := DefaultConfig()
		opts := &cfg.Channel.WithoutNamespace
		opts.HistorySize = 1
		opts.HistoryTTL = configtypes.Duration(time.Hour)
		opts.ForceRecoveryMode = "cache"
		opts.AutoCacheRecover = true
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "auto_cache_recover requires force_recovery")
	})

	t.Run("requires_cache_mode", func(t *testing.T) {
		cfg := DefaultConfig()
		opts := &cfg.Channel.WithoutNamespace
		opts.HistorySize = 1
		opts.HistoryTTL = configtypes.Duration(time.Hour)
		opts.ForceRecovery = true
		opts.AutoCacheRecover = true
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "force_recovery_mode set to cache")
	})
}

func TestValidateMapNamespace_Recoverable(t *testing.T) {
	t.Run("valid_minimal_defaults_zero", func(t *testing.T) {
		// Recoverable with all stream options at zero is valid
		// because centrifuge auto-derives defaults at runtime.
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "recoverable")
		ns.Map.KeyTTL = configtypes.Duration(30 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("valid_explicit_stream_options", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "recoverable")
		ns.Map.KeyTTL = configtypes.Duration(30 * time.Second)
		ns.Map.StreamSize = 200
		ns.Map.StreamTTL = configtypes.Duration(2 * time.Minute)
		ns.Map.MetaTTL = configtypes.Duration(20 * time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("missing_key_ttl", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "recoverable")
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map.key_ttl is required")
	})

	t.Run("meta_ttl_less_than_stream_ttl", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "recoverable")
		ns.Map.KeyTTL = configtypes.Duration(30 * time.Second)
		ns.Map.StreamTTL = configtypes.Duration(10 * time.Minute)
		ns.Map.MetaTTL = configtypes.Duration(5 * time.Minute) // less than stream_ttl
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "metadata must outlive stream")
	})

	t.Run("meta_ttl_equals_stream_ttl_ok", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "recoverable")
		ns.Map.KeyTTL = configtypes.Duration(30 * time.Second)
		ns.Map.StreamTTL = configtypes.Duration(5 * time.Minute)
		ns.Map.MetaTTL = configtypes.Duration(5 * time.Minute) // equals stream_ttl
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("only_stream_ttl_set_meta_ttl_zero_ok", func(t *testing.T) {
		// When only stream_ttl is set, meta_ttl=0 means auto-derived by centrifuge.
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "recoverable")
		ns.Map.KeyTTL = configtypes.Duration(30 * time.Second)
		ns.Map.StreamTTL = configtypes.Duration(5 * time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("only_meta_ttl_set_stream_ttl_zero_ok", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "recoverable")
		ns.Map.KeyTTL = configtypes.Duration(30 * time.Second)
		ns.Map.MetaTTL = configtypes.Duration(10 * time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("meta_ttl_less_than_auto_derived_stream_ttl", func(t *testing.T) {
		// StreamTTL=0 will be auto-derived to 1min by centrifuge.
		// MetaTTL=30s is less than that — must be caught at config time.
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "recoverable")
		ns.Map.KeyTTL = configtypes.Duration(30 * time.Second)
		ns.Map.MetaTTL = configtypes.Duration(30 * time.Second) // < auto-derived 1min
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "metadata must outlive stream")
	})

	t.Run("meta_ttl_equals_auto_derived_stream_ttl_ok", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "recoverable")
		ns.Map.KeyTTL = configtypes.Duration(30 * time.Second)
		ns.Map.MetaTTL = configtypes.Duration(1 * time.Minute) // == auto-derived 1min
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})
}

func TestValidateMapNamespace_Persistent(t *testing.T) {
	t.Run("valid_minimal", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("cp", "persistent")
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("valid_with_stream_options", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("cp", "persistent")
		ns.Map.StreamSize = 500
		ns.Map.StreamTTL = configtypes.Duration(5 * time.Minute)
		ns.Map.MetaTTL = configtypes.Duration(50 * time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("key_ttl_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("cp", "persistent")
		ns.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map.key_ttl must not be set")
	})

	t.Run("meta_ttl_less_than_stream_ttl", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("cp", "persistent")
		ns.Map.StreamTTL = configtypes.Duration(10 * time.Minute)
		ns.Map.MetaTTL = configtypes.Duration(1 * time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "metadata must outlive stream")
	})
}

func TestValidateMapNamespace_RequiredFields(t *testing.T) {
	t.Run("missing_mode", func(t *testing.T) {
		cfg := mapDefaultConfig()
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
			Name: "ns",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionType: "map",
				Map:              configtypes.MapConfig{},
			},
		}}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map.mode is required")
	})

	t.Run("invalid_mode", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "unknown_mode")
		ns.Map.KeyTTL = configtypes.Duration(30 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown map.mode")
	})

	t.Run("invalid_subscription_type", func(t *testing.T) {
		cfg := mapDefaultConfig()
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
			Name: "ns",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionType: "invalid_type",
			},
		}}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown subscription_type")
	})
}

func TestValidateMapNamespace_SubscriptionType(t *testing.T) {
	t.Run("map_clients", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "ephemeral")
		ns.SubscriptionType = "map_clients"
		ns.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("map_users", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "ephemeral")
		ns.SubscriptionType = "map_users"
		ns.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("stream_only_no_map_config_needed", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
			Name: "ns",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionType: "stream",
			},
		}}
		require.NoError(t, cfg.Validate())
	})

	t.Run("map_clients_persistent_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "persistent")
		ns.SubscriptionType = "map_clients"
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), `map.mode "persistent" is not allowed with subscription_type "map_clients"`)
	})

	t.Run("map_users_persistent_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "persistent")
		ns.SubscriptionType = "map_users"
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), `map.mode "persistent" is not allowed with subscription_type "map_users"`)
	})

	t.Run("client_key_with_publish_proxy_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "ephemeral")
		ns.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		ns.Map.ClientKey = "client_id"
		ns.Map.PublishProxyEnabled = true
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map.client_key is not compatible with map.publish_proxy_enabled")
	})

	t.Run("client_key_with_remove_proxy_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "ephemeral")
		ns.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		ns.Map.ClientKey = "user_id"
		ns.Map.RemoveProxyEnabled = true
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map.client_key is not compatible with")
	})
}

func TestValidateMapNamespace_PresenceAndRemoveOptions(t *testing.T) {
	t.Run("remove_on_unsubscribe_valid", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "ephemeral")
		ns.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		ns.Map.RemoveClientOnUnsubscribe = true
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("remove_on_unsubscribe_requires_map_type", func(t *testing.T) {
		cfg := mapDefaultConfig()
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
			Name: "ns",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionType: "stream",
				Map:              configtypes.MapConfig{RemoveClientOnUnsubscribe: true},
			},
		}}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map.remove_client_on_unsubscribe requires subscription_type to be a map type")
	})

	t.Run("map_clients_presence_channel_prefix_valid", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("games", "ephemeral")
		ns.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		ns.MapClientsPresenceChannelPrefix = "clients:"
		clients := mapNamespace("clients", "ephemeral")
		clients.SubscriptionType = "map_clients"
		clients.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns, clients}
		require.NoError(t, cfg.Validate())
	})

	t.Run("map_clients_presence_channel_prefix_not_found", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("games", "ephemeral")
		ns.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		ns.MapClientsPresenceChannelPrefix = "nonexistent:"
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_clients_presence_channel_prefix")
		require.Contains(t, err.Error(), "does not exist")
	})

	t.Run("map_users_presence_channel_prefix_not_found", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("games", "ephemeral")
		ns.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		ns.MapUsersPresenceChannelPrefix = "nonexistent:"
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_users_presence_channel_prefix")
		require.Contains(t, err.Error(), "does not exist")
	})

	t.Run("presence_channel_prefix_works_with_stream_type", func(t *testing.T) {
		cfg := mapDefaultConfig()
		clients := mapNamespace("clients", "ephemeral")
		clients.SubscriptionType = "map_clients"
		clients.Map.KeyTTL = configtypes.Duration(60 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
			Name: "ns",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionType:                "stream",
				MapClientsPresenceChannelPrefix: "clients:",
			},
		}, clients}
		require.NoError(t, cfg.Validate())
	})
}

func TestValidateMapNamespace_MapBrokerType(t *testing.T) {
	t.Run("unknown_type", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MapBroker.Type = "unknown"
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown map broker type")
	})

	for _, brokerType := range []string{"memory", "redis", "postgres"} {
		t.Run("valid_"+brokerType, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.MapBroker.Type = brokerType
			require.NoError(t, cfg.Validate())
		})
	}
}
