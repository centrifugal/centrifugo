package rule

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"github.com/stretchr/testify/require"
)

func TestChannelNotFound(t *testing.T) {
	c := DefaultConfig
	_, found, err := c.channelOpts("xxx")
	require.False(t, found)
	require.NoError(t, err)
}

func TestConfigValidateDefault(t *testing.T) {
	err := DefaultConfig.Validate()
	require.NoError(t, err)
}

func TestConfigValidateInvalidNamespaceName(t *testing.T) {
	c := DefaultConfig
	c.Namespaces = []ChannelNamespace{
		{
			Name:           "invalid name",
			ChannelOptions: ChannelOptions{},
		},
	}
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigCompiledChannelRegex(t *testing.T) {
	c := DefaultConfig
	c.ChannelRegex = "^test$"
	c.Namespaces = []ChannelNamespace{
		{
			Name: "name1",
			ChannelOptions: ChannelOptions{
				ChannelRegex: "^test_ns$",
			},
		},
		{
			Name:           "name2",
			ChannelOptions: ChannelOptions{},
		},
	}
	ruleContainer, err := NewContainer(c)
	require.NoError(t, err)

	require.NotNil(t, ruleContainer.Config().CompiledChannelRegex)
	require.NotNil(t, ruleContainer.Config().Namespaces[0].CompiledChannelRegex)
	require.Nil(t, ruleContainer.Config().Namespaces[1].CompiledChannelRegex)
}

func TestConfigValidateDuplicateNamespaceName(t *testing.T) {
	c := DefaultConfig
	c.Namespaces = []ChannelNamespace{
		{
			Name:           "name",
			ChannelOptions: ChannelOptions{},
		},
		{
			Name:           "name",
			ChannelOptions: ChannelOptions{},
		},
	}
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidateNoPersonalNamespace(t *testing.T) {
	c := DefaultConfig
	c.Namespaces = []ChannelNamespace{}
	c.UserSubscribeToPersonal = true
	c.UserPersonalChannelNamespace = "name"
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidatePersonalSingleConnectionMissingPresence(t *testing.T) {
	c := DefaultConfig
	c.Namespaces = []ChannelNamespace{}
	c.UserSubscribeToPersonal = true
	c.UserPersonalSingleConnection = true
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidatePersonalSingleConnectionOK(t *testing.T) {
	c := DefaultConfig
	c.Namespaces = []ChannelNamespace{}
	c.UserSubscribeToPersonal = true
	c.UserPersonalSingleConnection = true
	c.Presence = true
	err := c.Validate()
	require.NoError(t, err)
}

func TestConfigValidateHistoryTTL(t *testing.T) {
	t.Run("in_namespace", func(t *testing.T) {
		c := DefaultConfig
		c.Namespaces = []ChannelNamespace{
			{
				Name: "name1",
				ChannelOptions: ChannelOptions{
					HistorySize:    10,
					HistoryTTL:     tools.Duration(20 * time.Second),
					HistoryMetaTTL: tools.Duration(10 * time.Second),
				},
			},
		}
		err := c.Validate()
		require.ErrorContains(t, err, "history meta ttl")
	})
	t.Run("on_top_level", func(t *testing.T) {
		c := DefaultConfig
		c.HistorySize = 10
		c.HistoryTTL = tools.Duration(31 * 24 * time.Hour)
		err := c.Validate()
		require.ErrorContains(t, err, "history meta ttl")
	})
	t.Run("top_level_non_default_global", func(t *testing.T) {
		c := DefaultConfig
		c.GlobalHistoryMetaTTL = 10 * time.Hour
		c.HistorySize = 10
		c.HistoryTTL = tools.Duration(30 * 24 * time.Hour)
		err := c.Validate()
		require.ErrorContains(t, err, "history meta ttl")
	})
}

func TestConfigValidatePersonalSingleConnectionNamespacedFail(t *testing.T) {
	c := DefaultConfig
	c.Namespaces = []ChannelNamespace{}
	c.UserSubscribeToPersonal = true
	c.UserPersonalSingleConnection = true
	c.UserPersonalChannelNamespace = "public"
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidatePersonalSingleConnectionNamespacedOK(t *testing.T) {
	c := DefaultConfig
	c.Namespaces = []ChannelNamespace{}
	c.UserSubscribeToPersonal = true
	c.UserPersonalSingleConnection = true
	c.UserPersonalChannelNamespace = "public"
	c.Namespaces = []ChannelNamespace{{
		Name: "public",
		ChannelOptions: ChannelOptions{
			Presence: true,
		},
	}}
	err := c.Validate()
	require.NoError(t, err)
}

func TestConfigValidateMalformedReceiverTopLevel(t *testing.T) {
	c := DefaultConfig
	c.ForceRecovery = true
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidateMalformedReceiverInNamespace(t *testing.T) {
	c := DefaultConfig
	c.Namespaces = []ChannelNamespace{
		{
			Name: "name",
			ChannelOptions: ChannelOptions{
				ForceRecovery: true,
			},
		},
	}
	err := c.Validate()
	require.Error(t, err)
}

func TestUserAllowed(t *testing.T) {
	rules, err := NewContainer(DefaultConfig)
	require.NoError(t, err)
	require.True(t, rules.UserAllowed("channel#1", "1"))
	require.True(t, rules.UserAllowed("channel", "1"))
	require.False(t, rules.UserAllowed("channel#1", "2"))
	require.True(t, rules.UserAllowed("channel#1,2", "1"))
	require.True(t, rules.UserAllowed("channel#1,2", "2"))
	require.False(t, rules.UserAllowed("channel#1,2", "3"))
}

func TestIsUserLimited(t *testing.T) {
	rules, err := NewContainer(DefaultConfig)
	require.NoError(t, err)
	require.True(t, rules.IsUserLimited("#12"))
	require.True(t, rules.IsUserLimited("test#12"))
	config := rules.Config()
	config.ChannelUserBoundary = ""
	err = rules.Reload(config)
	require.NoError(t, err)
	require.False(t, rules.IsUserLimited("#12"))
}

func BenchmarkContainer_ChannelOptions(b *testing.B) {
	cfg := DefaultConfig

	const numNamespaces = 128

	var channels []string

	var namespaces []ChannelNamespace
	for i := 0; i < numNamespaces; i++ {
		namespaces = append(namespaces, ChannelNamespace{
			Name: "test" + strconv.Itoa(i),
		})
		channels = append(channels, "test"+strconv.Itoa(i)+":123")
	}
	cfg.Namespaces = namespaces

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
	cfg := DefaultConfig
	var namespaces []ChannelNamespace
	for i := 0; i < 100; i++ {
		namespaces = append(namespaces, ChannelNamespace{
			Name: "test" + strconv.Itoa(i),
		})
	}
	cfg.Namespaces = namespaces
	c, _ := NewContainer(cfg)
	c.ChannelOptionsCacheTTL = 200 * time.Millisecond

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			testConfig = c.Config()
			if len(testConfig.Namespaces) != 100 {
				b.Fatal("wrong config")
			}
		}
	})
}
