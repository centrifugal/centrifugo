package app

import (
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/stretchr/testify/require"
)

func TestRawModeDataCheck(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		{Name: "json", ChannelOptions: configtypes.ChannelOptions{PublicationDataFormat: configtypes.PublicationDataFormatJSON}},
		{Name: "object", ChannelOptions: configtypes.ChannelOptions{PublicationDataFormat: configtypes.PublicationDataFormatJSONObject}},
		{Name: "binary", ChannelOptions: configtypes.ChannelOptions{PublicationDataFormat: configtypes.PublicationDataFormatBinary}},
		{Name: "any"},
	}
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	check := rawModeDataCheck(cfgContainer)

	tests := []struct {
		channel string
		data    string
		ok      bool
	}{
		{"json:1", `{"a":1}`, true},
		{"json:1", `[1]`, true},
		{"json:1", `not json`, false},
		{"json:1", ``, false},
		{"object:1", `{"a":1}`, true},
		{"object:1", `[1]`, false},
		{"binary:1", "\x00\x01", true},
		{"binary:1", ``, true},
		// No format set: raw mode passes on whatever comes, as it always did.
		{"any:1", "\x00\x01", true},
		{"any:1", ``, true},
		{"no_namespace", `not json`, true},
		{"unknown:1", `not json`, true},
	}
	for _, tt := range tests {
		err := check(tt.channel, []byte(tt.data))
		require.Equal(t, tt.ok, err == nil, "channel %s, data %q: %v", tt.channel, tt.data, err)
	}

	// The global format is the default of every namespace.
	cfg.Channel.PublicationDataFormat = configtypes.PublicationDataFormatJSON
	cfgContainer, err = config.NewContainer(cfg)
	require.NoError(t, err)
	check = rawModeDataCheck(cfgContainer)
	require.Error(t, check("any:1", []byte(`not json`)))
	require.Error(t, check("no_namespace", []byte(`not json`)))
	require.NoError(t, check("binary:1", []byte(`not json`)))
}
