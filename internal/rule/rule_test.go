package rule

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelNotFound(t *testing.T) {
	c := DefaultRuleConfig
	_, found, err := c.channelOpts("xxx")
	require.False(t, found)
	require.NoError(t, err)
}

func TestConfigValidateDefault(t *testing.T) {
	err := DefaultRuleConfig.Validate()
	require.NoError(t, err)
}

func TestConfigValidateInvalidNamespaceName(t *testing.T) {
	c := DefaultRuleConfig
	c.Namespaces = []ChannelNamespace{
		{
			Name:                    "invalid name",
			NamespaceChannelOptions: NamespaceChannelOptions{},
		},
	}
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidateDuplicateNamespaceName(t *testing.T) {
	c := DefaultRuleConfig
	c.Namespaces = []ChannelNamespace{
		{
			Name:                    "name",
			NamespaceChannelOptions: NamespaceChannelOptions{},
		},
		{
			Name:                    "name",
			NamespaceChannelOptions: NamespaceChannelOptions{},
		},
	}
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidateNoPersonalNamespace(t *testing.T) {
	c := DefaultRuleConfig
	c.Namespaces = []ChannelNamespace{}
	c.UserSubscribeToPersonal = true
	c.UserPersonalChannelNamespace = "name"
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidateMalformedReceiverTopLevel(t *testing.T) {
	c := DefaultRuleConfig
	c.HistoryRecover = true
	err := c.Validate()
	require.Error(t, err)
}

func TestConfigValidateMalformedReceiverInNamespace(t *testing.T) {
	c := DefaultRuleConfig
	c.Namespaces = []ChannelNamespace{
		{
			Name: "name",
			NamespaceChannelOptions: NamespaceChannelOptions{
				HistoryRecover: true,
			},
		},
	}
	err := c.Validate()
	require.Error(t, err)
}

func TestUserAllowed(t *testing.T) {
	rules := NewNamespaceRuleContainer(DefaultRuleConfig)
	require.True(t, rules.UserAllowed("channel#1", "1"))
	require.True(t, rules.UserAllowed("channel", "1"))
	require.False(t, rules.UserAllowed("channel#1", "2"))
	require.True(t, rules.UserAllowed("channel#1,2", "1"))
	require.True(t, rules.UserAllowed("channel#1,2", "2"))
	require.False(t, rules.UserAllowed("channel#1,2", "3"))
}

func TestIsUserLimited(t *testing.T) {
	rules := NewNamespaceRuleContainer(DefaultRuleConfig)
	require.True(t, rules.IsUserLimited("#12"))
	require.True(t, rules.IsUserLimited("test#12"))
	rules.config.ChannelUserBoundary = ""
	require.False(t, rules.IsUserLimited("#12"))
}
