package config

import (
	"slices"
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

func TestValidChannelName(t *testing.T) {
	c := defaultConfig(t)
	c.Channel.Namespaces = []configtypes.ChannelNamespace{
		{
			Name: "digits",
			ChannelOptions: configtypes.ChannelOptions{
				ChannelRegex: `^\d+$`,
			},
		},
		{
			Name:           "plain",
			ChannelOptions: configtypes.ChannelOptions{},
		},
		{
			Name: "any",
			ChannelOptions: configtypes.ChannelOptions{
				ChannelRegex: `^.+$`,
			},
		},
	}
	container, err := NewContainer(c)
	require.NoError(t, err)

	tests := []struct {
		name    string
		channel string
		valid   bool
	}{
		{"no regex, ascii channel", "plain:index", true},
		{"no regex, non-ascii channel", "plain:индекс", false},
		{"regex matches rest, not the whole channel", "digits:42", true},
		{"regex does not match rest", "digits:abc", false},
		{"regex replaces the ascii check", "any:индекс", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, rest, chOpts, found, err := container.ChannelOptions(tt.channel)
			require.NoError(t, err)
			require.True(t, found)
			valid, err := container.ValidChannelName(tt.channel, rest, chOpts)
			require.NoError(t, err)
			require.Equal(t, tt.valid, valid)
		})
	}
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

func TestPublicationDataFormatInheritance(t *testing.T) {
	t.Run("namespace inherits global format", func(t *testing.T) {
		c := defaultConfig(t)
		c.Channel.PublicationDataFormat = "json"
		c.Channel.Namespaces = []configtypes.ChannelNamespace{
			{
				Name:           "test",
				ChannelOptions: configtypes.ChannelOptions{
					// No PublicationDataFormat set
				},
			},
		}
		container, err := NewContainer(c)
		require.NoError(t, err)

		// Channel without namespace should use global format
		_, _, chOpts, ok, err := container.ChannelOptions("mychannel")
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "json", chOpts.PublicationDataFormat)

		// Namespace without explicit format should inherit global
		_, _, chOpts, ok, err = container.ChannelOptions("test:mychannel")
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "json", chOpts.PublicationDataFormat)
	})

	t.Run("namespace overrides global format", func(t *testing.T) {
		c := defaultConfig(t)
		c.Channel.PublicationDataFormat = "json"
		c.Channel.Namespaces = []configtypes.ChannelNamespace{
			{
				Name: "test",
				ChannelOptions: configtypes.ChannelOptions{
					PublicationDataFormat: "binary",
				},
			},
		}
		container, err := NewContainer(c)
		require.NoError(t, err)

		// Namespace with explicit format should use its own
		_, _, chOpts, ok, err := container.ChannelOptions("test:mychannel")
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "binary", chOpts.PublicationDataFormat)
	})

	t.Run("no global format set", func(t *testing.T) {
		c := defaultConfig(t)
		// No global format set
		c.Channel.Namespaces = []configtypes.ChannelNamespace{
			{
				Name:           "test",
				ChannelOptions: configtypes.ChannelOptions{
					// No PublicationDataFormat set
				},
			},
		}
		container, err := NewContainer(c)
		require.NoError(t, err)

		// Should use empty string (default behavior)
		_, _, chOpts, ok, err := container.ChannelOptions("test:mychannel")
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "", chOpts.PublicationDataFormat)
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

func TestChannelOptionsRef(t *testing.T) {
	newContainer := func(t *testing.T, historySize int) *Container {
		c := defaultConfig(t)
		c.Channel.PublicationDataFormat = "json"
		c.Channel.Namespaces = []configtypes.ChannelNamespace{{
			Name:           "test",
			ChannelOptions: configtypes.ChannelOptions{HistorySize: historySize, HistoryTTL: configtypes.Duration(time.Minute)},
		}}
		container, err := NewContainer(c)
		require.NoError(t, err)
		return container
	}

	container := newContainer(t, 10)

	// The same options as ChannelOptions, the inherited global publication
	// data format included.
	for _, ch := range []string{"test:1", "mychannel"} {
		ref, ok, err := container.ChannelOptionsRef(ch)
		require.NoError(t, err)
		require.True(t, ok)
		_, _, chOpts, _, _ := container.ChannelOptions(ch)
		require.Equal(t, chOpts, *ref)
		require.Equal(t, "json", ref.PublicationDataFormat)
	}

	_, ok, err := container.ChannelOptionsRef("unknown:1")
	require.NoError(t, err)
	require.False(t, ok)

	t.Run("channels of a namespace share the options", func(t *testing.T) {
		first, _, _ := container.ChannelOptionsRef("test:1")
		second, _, _ := container.ChannelOptionsRef("test:2")
		require.Same(t, first, second)
	})

	t.Run("reload", func(t *testing.T) {
		container := newContainer(t, 10)
		before, _, _ := container.ChannelOptionsRef("test:1")

		cfg := container.Config()
		// A copy of the namespaces: Config shares them with the live config.
		cfg.Channel.Namespaces = slices.Clone(cfg.Channel.Namespaces)
		cfg.Channel.Namespaces[0].HistorySize = 20
		require.NoError(t, container.Reload(cfg))

		after, _, _ := container.ChannelOptionsRef("test:1")
		require.Equal(t, 20, after.HistorySize)
		// Options handed out before the reload are left as they were.
		require.Equal(t, 10, before.HistorySize)
	})
}

func BenchmarkContainer_ChannelOptionsRef(b *testing.B) {
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

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			i++
			if _, ok, _ := c.ChannelOptionsRef(channels[i%numNamespaces]); !ok {
				b.Fatal("ns not found")
			}
		}
	})
}

// BenchmarkContainer_ChannelOptionsRefManyChannels looks up many distinct
// channels in turn, as a broadcast into thousands of channels or a server with
// many personal channels does.
func BenchmarkContainer_ChannelOptionsRefManyChannels(b *testing.B) {
	cfg := defaultConfig(b)
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{Name: "user"}}
	c, _ := NewContainer(cfg)

	channels := make([]string, 10000)
	for i := range channels {
		channels[i] = "user:" + strconv.Itoa(i)
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			i++
			if _, ok, _ := c.ChannelOptionsRef(channels[i%len(channels)]); !ok {
				b.Fatal("ns not found")
			}
		}
	})
}

// TestChannelOptionsLookupAllocs checks looking up channel options does not
// allocate, however many different channels are looked up. A broadcast into
// thousands of channels looks up each of them.
func TestChannelOptionsLookupAllocs(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{Name: "user"}}
	container, err := NewContainer(cfg)
	require.NoError(t, err)

	channels := make([]string, 1000)
	for i := range channels {
		if i%2 == 0 {
			channels[i] = "user:" + strconv.Itoa(i)
		} else {
			channels[i] = "channel" + strconv.Itoa(i)
		}
	}
	i := 0
	allocs := testing.AllocsPerRun(500, func() {
		_, ok, _ := container.ChannelOptionsRef(channels[i%len(channels)])
		require.True(t, ok)
		_, _, _, ok, _ = container.ChannelOptions(channels[i%len(channels)])
		require.True(t, ok)
		i++
	})
	require.Zero(t, allocs)
}

// TestChannelOptionsMatchNamespaceSearch checks looking up precomputed
// options finds the same namespace, rest and options as searching the
// namespaces does, for channel names at the edges of the naming rules.
func TestChannelOptionsMatchNamespaceSearch(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Channel.PublicationDataFormat = "json"
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		{Name: "ns", ChannelOptions: configtypes.ChannelOptions{HistorySize: 1, HistoryTTL: configtypes.Duration(time.Minute)}},
		{Name: "other", ChannelOptions: configtypes.ChannelOptions{PublicationDataFormat: "binary"}},
	}
	container, err := NewContainer(cfg)
	require.NoError(t, err)
	prepared := container.Config()

	channels := []string{
		"ns:a", "ns:a:b", "ns:", ":a", "a", "", "$ns:a", "$a", "$", "other:x",
		"unknown:x", "ns", "a#1", "ns:a#1",
	}
	for _, ch := range channels {
		// What searching the namespaces gives, as lookups worked before.
		wantNs, wantRest := "", ch
		trimmed := strings.TrimPrefix(ch, prepared.Channel.PrivatePrefix)
		if strings.Contains(trimmed, prepared.Channel.NamespaceBoundary) {
			parts := strings.SplitN(trimmed, prepared.Channel.NamespaceBoundary, 2)
			wantNs, wantRest = parts[0], parts[1]
		}
		wantOpts, wantOk, err := channelOpts(&prepared, wantNs)
		require.NoError(t, err)
		if wantOk && wantOpts.PublicationDataFormat == "" {
			wantOpts.PublicationDataFormat = prepared.Channel.PublicationDataFormat
		}

		nsName, rest, chOpts, ok, err := container.ChannelOptions(ch)
		require.NoError(t, err)
		require.Equal(t, wantNs, nsName, "channel %q", ch)
		require.Equal(t, wantRest, rest, "channel %q", ch)
		require.Equal(t, wantOk, ok, "channel %q", ch)
		if ok {
			require.Equal(t, wantOpts, chOpts, "channel %q", ch)
		}
	}
}
