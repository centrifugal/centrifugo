package config

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/stretchr/testify/require"
)

func defaultConfig(tb testing.TB) Config {
	conf := DefaultConfig()
	require.NotNil(tb, conf)
	return conf
}

func TestChannelNotFound(t *testing.T) {
	c := defaultConfig(t)
	_, found, err := channelOpts(&c, "xxx")
	require.False(t, found)
	require.NoError(t, err)
}

func TestConfigValidateDefault(t *testing.T) {
	err := defaultConfig(t).Validate()
	require.NoError(t, err)
}

func TestConfigValidateInvalidNamespaceName(t *testing.T) {
	c := defaultConfig(t)
	c.Channel.Namespaces = []configtypes.ChannelNamespace{
		{
			Name:           "invalid name",
			ChannelOptions: configtypes.ChannelOptions{},
		},
	}
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigCompiledChannelRegex(t *testing.T) {
	c := defaultConfig(t)
	c.Channel.WithoutNamespace.ChannelRegex = "^test$"
	c.Channel.Namespaces = []configtypes.ChannelNamespace{
		{
			Name: "name1",
			ChannelOptions: configtypes.ChannelOptions{
				ChannelRegex: "^test_ns$",
			},
		},
		{
			Name:           "name2",
			ChannelOptions: configtypes.ChannelOptions{},
		},
	}
	ruleContainer, err := NewContainer(c)
	require.NoError(t, err)

	require.NotNil(t, ruleContainer.Config().Channel.WithoutNamespace.CompiledChannelRegex)
	require.NotNil(t, ruleContainer.Config().Channel.Namespaces[0].CompiledChannelRegex)
	require.Nil(t, ruleContainer.Config().Channel.Namespaces[1].CompiledChannelRegex)
}

func TestConfigValidateDuplicateNamespaceName(t *testing.T) {
	c := defaultConfig(t)
	c.Channel.Namespaces = []configtypes.ChannelNamespace{
		{
			Name:           "name",
			ChannelOptions: configtypes.ChannelOptions{},
		},
		{
			Name:           "name",
			ChannelOptions: configtypes.ChannelOptions{},
		},
	}
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidateNoPersonalNamespace(t *testing.T) {
	c := defaultConfig(t)
	c.Channel.Namespaces = []configtypes.ChannelNamespace{}
	c.Client.SubscribeToUserPersonalChannel.Enabled = true
	c.Client.SubscribeToUserPersonalChannel.PersonalChannelNamespace = "name"
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidatePersonalSingleConnectionMissingPresence(t *testing.T) {
	c := defaultConfig(t)
	c.Channel.Namespaces = []configtypes.ChannelNamespace{}
	c.Client.SubscribeToUserPersonalChannel.Enabled = true
	c.Client.SubscribeToUserPersonalChannel.SingleConnection = true
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidatePersonalSingleConnectionOK(t *testing.T) {
	c := defaultConfig(t)
	c.Channel.Namespaces = []configtypes.ChannelNamespace{}
	c.Client.SubscribeToUserPersonalChannel.Enabled = true
	c.Client.SubscribeToUserPersonalChannel.SingleConnection = true
	c.Channel.WithoutNamespace.Presence = true
	err := c.Validate()
	require.NoError(t, err)
}

func TestConfigValidateHistoryTTL(t *testing.T) {
	t.Run("in_namespace", func(t *testing.T) {
		c := defaultConfig(t)
		c.Channel.Namespaces = []configtypes.ChannelNamespace{
			{
				Name: "name1",
				ChannelOptions: configtypes.ChannelOptions{
					HistorySize:    10,
					HistoryTTL:     configtypes.Duration(20 * time.Second),
					HistoryMetaTTL: configtypes.Duration(10 * time.Second),
				},
			},
		}
		err := c.Validate()
		require.ErrorContains(t, err, "history meta ttl")
	})
	t.Run("on_top_level", func(t *testing.T) {
		c := defaultConfig(t)
		c.Channel.WithoutNamespace.HistorySize = 10
		c.Channel.WithoutNamespace.HistoryTTL = configtypes.Duration(31 * 24 * time.Hour)
		err := c.Validate()
		require.ErrorContains(t, err, "history meta ttl")
	})
	t.Run("top_level_non_default_global", func(t *testing.T) {
		c := defaultConfig(t)
		c.Channel.HistoryMetaTTL = configtypes.Duration(10 * time.Hour)
		c.Channel.WithoutNamespace.HistorySize = 10
		c.Channel.WithoutNamespace.HistoryTTL = configtypes.Duration(30 * 24 * time.Hour)
		err := c.Validate()
		require.ErrorContains(t, err, "history meta ttl")
	})
}

func TestConfigValidatePersonalSingleConnectionNamespacedFail(t *testing.T) {
	c := defaultConfig(t)
	c.Channel.Namespaces = []configtypes.ChannelNamespace{}
	c.Client.SubscribeToUserPersonalChannel.Enabled = true
	c.Client.SubscribeToUserPersonalChannel.SingleConnection = true
	c.Client.SubscribeToUserPersonalChannel.PersonalChannelNamespace = "public"
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidatePersonalSingleConnectionNamespacedOK(t *testing.T) {
	c := defaultConfig(t)
	c.Channel.Namespaces = []configtypes.ChannelNamespace{}
	c.Client.SubscribeToUserPersonalChannel.Enabled = true
	c.Client.SubscribeToUserPersonalChannel.SingleConnection = true
	c.Client.SubscribeToUserPersonalChannel.PersonalChannelNamespace = "public"
	c.Channel.Namespaces = []configtypes.ChannelNamespace{{
		Name: "public",
		ChannelOptions: configtypes.ChannelOptions{
			Presence: true,
		},
	}}
	err := c.Validate()
	require.NoError(t, err)
}

func TestConfigValidateMalformedRecoveryTopLevel(t *testing.T) {
	c := defaultConfig(t)
	c.Channel.WithoutNamespace.ForceRecovery = true
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidateMalformedRecoveryInNamespace(t *testing.T) {
	c := defaultConfig(t)
	c.Channel.Namespaces = []configtypes.ChannelNamespace{
		{
			Name: "name",
			ChannelOptions: configtypes.ChannelOptions{
				ForceRecovery: true,
			},
		},
	}
	err := c.Validate()
	require.Error(t, err)
}

func TestUserAllowed(t *testing.T) {
	rules, err := NewContainer(defaultConfig(t))
	require.NoError(t, err)
	require.True(t, rules.UserAllowed("channel#1", "1"))
	require.True(t, rules.UserAllowed("channel", "1"))
	require.False(t, rules.UserAllowed("channel#1", "2"))
	require.True(t, rules.UserAllowed("channel#1,2", "1"))
	require.True(t, rules.UserAllowed("channel#1,2", "2"))
	require.False(t, rules.UserAllowed("channel#1,2", "3"))
}

func TestIsUserLimited(t *testing.T) {
	rules, err := NewContainer(defaultConfig(t))
	require.NoError(t, err)
	require.True(t, rules.IsUserLimited("#12"))
	require.True(t, rules.IsUserLimited("test#12"))
	config := rules.Config()
	config.Channel.UserBoundary = ""
	err = rules.Reload(config)
	require.NoError(t, err)
	require.False(t, rules.IsUserLimited("#12"))
}

func BenchmarkContainer_ChannelOptions(b *testing.B) {
	cfg := defaultConfig(b)

	const numNamespaces = 128

	var channels []string

	var namespaces []configtypes.ChannelNamespace
	for i := 0; i < numNamespaces; i++ {
		namespaces = append(namespaces, configtypes.ChannelNamespace{
			Name: "test" + strconv.Itoa(i),
		})
		channels = append(channels, "test"+strconv.Itoa(i)+":123")
	}
	cfg.Channel.Namespaces = namespaces

	c, _ := NewContainer(cfg)
	c.ChannelOptionsCacheTTL = 200 * time.Millisecond

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			i++
			ch := channels[i%numNamespaces]
			nsName, _, _, ok, _ := c.ChannelOptions(ch)
			if !ok {
				b.Fatal("ns not found")
			}
			if !strings.HasPrefix(ch, nsName) {
				b.Fatal("wrong ns name: " + nsName)
			}
		}
	})
}

var testConfig Config

func BenchmarkContainer_Config(b *testing.B) {
	cfg := defaultConfig(b)
	var namespaces []configtypes.ChannelNamespace
	for i := 0; i < 100; i++ {
		namespaces = append(namespaces, configtypes.ChannelNamespace{
			Name: "test" + strconv.Itoa(i),
		})
	}
	cfg.Channel.Namespaces = namespaces
	c, _ := NewContainer(cfg)
	c.ChannelOptionsCacheTTL = 200 * time.Millisecond

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			testConfig = c.Config()
			if len(testConfig.Channel.Namespaces) != 100 {
				b.Fatal("wrong config")
			}
		}
	})
}
