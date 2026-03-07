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

func mapNamespace(name string, syncMode, retentionMode string) configtypes.ChannelNamespace {
	return configtypes.ChannelNamespace{
		Name: name,
		ChannelOptions: configtypes.ChannelOptions{
			SubscriptionTypes: []string{"map"},
			MapSyncMode:       syncMode,
			MapRetentionMode:  retentionMode,
		},
	}
}

func TestValidateMapNamespace_EphemeralExpiring(t *testing.T) {
	t.Run("valid_minimal", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ee", "ephemeral", "expiring")
		ns.MapKeyTTL = configtypes.Duration(60 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("missing_key_ttl", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ee", "ephemeral", "expiring")
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_key_ttl is required")
	})

	t.Run("stream_size_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ee", "ephemeral", "expiring")
		ns.MapKeyTTL = configtypes.Duration(60 * time.Second)
		ns.MapStreamSize = 100
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_stream_size must be 0")
	})

	t.Run("stream_ttl_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ee", "ephemeral", "expiring")
		ns.MapKeyTTL = configtypes.Duration(60 * time.Second)
		ns.MapStreamTTL = configtypes.Duration(time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_stream_ttl must be 0")
	})

	t.Run("meta_ttl_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ee", "ephemeral", "expiring")
		ns.MapKeyTTL = configtypes.Duration(60 * time.Second)
		ns.MapMetaTTL = configtypes.Duration(10 * time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_meta_ttl must be 0")
	})
}

func TestValidateMapNamespace_EphemeralPermanent(t *testing.T) {
	t.Run("valid_minimal", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ep", "ephemeral", "permanent")
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("key_ttl_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ep", "ephemeral", "permanent")
		ns.MapKeyTTL = configtypes.Duration(60 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_key_ttl must not be set")
	})

	t.Run("stream_options_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ep", "ephemeral", "permanent")
		ns.MapStreamSize = 50
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_stream_size must be 0")
	})
}

func TestValidateMapNamespace_ConvergingExpiring(t *testing.T) {
	t.Run("valid_minimal_defaults_zero", func(t *testing.T) {
		// Converging+expiring with all stream options at zero is valid
		// because centrifuge auto-derives defaults at runtime.
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "converging", "expiring")
		ns.MapKeyTTL = configtypes.Duration(30 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("valid_explicit_stream_options", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "converging", "expiring")
		ns.MapKeyTTL = configtypes.Duration(30 * time.Second)
		ns.MapStreamSize = 200
		ns.MapStreamTTL = configtypes.Duration(2 * time.Minute)
		ns.MapMetaTTL = configtypes.Duration(20 * time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("missing_key_ttl", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "converging", "expiring")
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_key_ttl is required")
	})

	t.Run("meta_ttl_less_than_stream_ttl", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "converging", "expiring")
		ns.MapKeyTTL = configtypes.Duration(30 * time.Second)
		ns.MapStreamTTL = configtypes.Duration(10 * time.Minute)
		ns.MapMetaTTL = configtypes.Duration(5 * time.Minute) // less than stream_ttl
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "metadata must outlive stream")
	})

	t.Run("meta_ttl_equals_stream_ttl_ok", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "converging", "expiring")
		ns.MapKeyTTL = configtypes.Duration(30 * time.Second)
		ns.MapStreamTTL = configtypes.Duration(5 * time.Minute)
		ns.MapMetaTTL = configtypes.Duration(5 * time.Minute) // equals stream_ttl
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("only_stream_ttl_set_meta_ttl_zero_ok", func(t *testing.T) {
		// When only stream_ttl is set, meta_ttl=0 means auto-derived by centrifuge.
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "converging", "expiring")
		ns.MapKeyTTL = configtypes.Duration(30 * time.Second)
		ns.MapStreamTTL = configtypes.Duration(5 * time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("only_meta_ttl_set_stream_ttl_zero_ok", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "converging", "expiring")
		ns.MapKeyTTL = configtypes.Duration(30 * time.Second)
		ns.MapMetaTTL = configtypes.Duration(10 * time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("meta_ttl_less_than_auto_derived_stream_ttl", func(t *testing.T) {
		// StreamTTL=0 will be auto-derived to 1min by centrifuge.
		// MetaTTL=30s is less than that — must be caught at config time.
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "converging", "expiring")
		ns.MapKeyTTL = configtypes.Duration(30 * time.Second)
		ns.MapMetaTTL = configtypes.Duration(30 * time.Second) // < auto-derived 1min
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "metadata must outlive stream")
	})

	t.Run("meta_ttl_equals_auto_derived_stream_ttl_ok", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ce", "converging", "expiring")
		ns.MapKeyTTL = configtypes.Duration(30 * time.Second)
		ns.MapMetaTTL = configtypes.Duration(1 * time.Minute) // == auto-derived 1min
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})
}

func TestValidateMapNamespace_ConvergingPermanent(t *testing.T) {
	t.Run("valid_minimal", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("cp", "converging", "permanent")
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("valid_with_stream_options", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("cp", "converging", "permanent")
		ns.MapStreamSize = 500
		ns.MapStreamTTL = configtypes.Duration(5 * time.Minute)
		ns.MapMetaTTL = configtypes.Duration(50 * time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("key_ttl_rejected", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("cp", "converging", "permanent")
		ns.MapKeyTTL = configtypes.Duration(60 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_key_ttl must not be set")
	})

	t.Run("meta_ttl_less_than_stream_ttl", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("cp", "converging", "permanent")
		ns.MapStreamTTL = configtypes.Duration(10 * time.Minute)
		ns.MapMetaTTL = configtypes.Duration(1 * time.Minute)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "metadata must outlive stream")
	})
}

func TestValidateMapNamespace_RequiredFields(t *testing.T) {
	t.Run("missing_sync_mode", func(t *testing.T) {
		cfg := mapDefaultConfig()
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
			Name: "ns",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionTypes: []string{"map"},
				MapRetentionMode:  "expiring",
				MapKeyTTL:         configtypes.Duration(30 * time.Second),
			},
		}}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_sync_mode is required")
	})

	t.Run("missing_retention_mode", func(t *testing.T) {
		cfg := mapDefaultConfig()
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
			Name: "ns",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionTypes: []string{"map"},
				MapSyncMode:       "ephemeral",
			},
		}}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_retention_mode is required")
	})

	t.Run("invalid_sync_mode", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "unknown_mode", "expiring")
		ns.MapKeyTTL = configtypes.Duration(30 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown map_sync_mode")
	})

	t.Run("invalid_retention_mode", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "ephemeral", "unknown_mode")
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown map_retention_mode")
	})

	t.Run("invalid_subscription_type", func(t *testing.T) {
		cfg := mapDefaultConfig()
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
			Name: "ns",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionTypes: []string{"invalid_type"},
			},
		}}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown subscription type")
	})
}

func TestValidateMapNamespace_SubscriptionTypes(t *testing.T) {
	t.Run("map_clients", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "ephemeral", "expiring")
		ns.SubscriptionTypes = []string{"map_clients"}
		ns.MapKeyTTL = configtypes.Duration(60 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("map_users", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "ephemeral", "expiring")
		ns.SubscriptionTypes = []string{"map_users"}
		ns.MapKeyTTL = configtypes.Duration(60 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("mixed_map_and_stream", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "converging", "permanent")
		ns.SubscriptionTypes = []string{"stream", "map", "map_clients"}
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("stream_only_no_map_config_needed", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
			Name: "ns",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionTypes: []string{"stream"},
			},
		}}
		require.NoError(t, cfg.Validate())
	})
}

func TestValidateMapNamespace_PresenceAndRemoveOptions(t *testing.T) {
	t.Run("remove_on_unsubscribe_valid", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("ns", "ephemeral", "expiring")
		ns.MapKeyTTL = configtypes.Duration(60 * time.Second)
		ns.MapRemoveOnUnsubscribe = true
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		require.NoError(t, cfg.Validate())
	})

	t.Run("remove_on_unsubscribe_requires_map_type", func(t *testing.T) {
		cfg := mapDefaultConfig()
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
			Name: "ns",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionTypes:      []string{"stream"},
				MapRemoveOnUnsubscribe: true,
			},
		}}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_remove_on_unsubscribe requires subscription_types to include map types")
	})

	t.Run("client_presence_namespace_valid", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("games", "ephemeral", "expiring")
		ns.MapKeyTTL = configtypes.Duration(60 * time.Second)
		ns.MapClientPresenceNamespace = "clients"
		clients := mapNamespace("clients", "ephemeral", "expiring")
		clients.MapKeyTTL = configtypes.Duration(60 * time.Second)
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns, clients}
		require.NoError(t, cfg.Validate())
	})

	t.Run("client_presence_namespace_not_found", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("games", "ephemeral", "expiring")
		ns.MapKeyTTL = configtypes.Duration(60 * time.Second)
		ns.MapClientPresenceNamespace = "nonexistent"
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_client_presence_namespace \"nonexistent\" not found")
	})

	t.Run("user_presence_namespace_not_found", func(t *testing.T) {
		cfg := mapDefaultConfig()
		ns := mapNamespace("games", "ephemeral", "expiring")
		ns.MapKeyTTL = configtypes.Duration(60 * time.Second)
		ns.MapUserPresenceNamespace = "nonexistent"
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{ns}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_user_presence_namespace \"nonexistent\" not found")
	})

	t.Run("presence_namespace_requires_map_type", func(t *testing.T) {
		cfg := mapDefaultConfig()
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
			Name: "ns",
			ChannelOptions: configtypes.ChannelOptions{
				SubscriptionTypes:          []string{"stream"},
				MapClientPresenceNamespace: "clients",
			},
		}}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "map_client_presence_namespace requires subscription_types to include map types")
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
