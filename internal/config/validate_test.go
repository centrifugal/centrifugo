package config

import (
	"testing"

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
