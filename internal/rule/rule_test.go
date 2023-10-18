package rule

import (
	"testing"

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
	cfg.Namespaces = []ChannelNamespace{
		{
			Name: "test0",
		},
		{
			Name: "test1",
		},
		{
			Name: "test2",
		},
		{
			Name: "test3",
		},
		{
			Name: "test4",
		},
		{
			Name: "test5",
		},
		{
			Name: "test6",
		},
		{
			Name: "test7",
		},
		{
			Name: "test8",
		},
		{
			Name: "test9",
		},
	}
	c, _ := NewContainer(cfg)

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			nsName, _, _, ok, _ := c.ChannelOptions("test9:123")
			if !ok {
				b.Fatal("ns not found")
			}
			if nsName != "test9" {
				b.Fatal("wrong ns name: " + nsName)
			}
		}
	})
}
