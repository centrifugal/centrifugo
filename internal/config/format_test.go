package config

import (
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/stretchr/testify/require"
)

func TestValidatePublicationData(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		format  string
		wantErr bool
		errMsg  string
	}{
		// Default format tests
		{
			name:    "default format - non-empty data allowed",
			data:    []byte("any data"),
			format:  "",
			wantErr: false,
		},
		{
			name:    "default format - empty data rejected",
			data:    []byte{},
			format:  "",
			wantErr: true,
			errMsg:  "data required",
		},
		// JSON format tests
		{
			name:    "json format - valid json object",
			data:    []byte(`{"key": "value"}`),
			format:  configtypes.PublicationDataFormatJSON,
			wantErr: false,
		},
		{
			name:    "json format - valid json array",
			data:    []byte(`[1, 2, 3]`),
			format:  configtypes.PublicationDataFormatJSON,
			wantErr: false,
		},
		{
			name:    "json format - valid json primitives",
			data:    []byte(`"string"`),
			format:  configtypes.PublicationDataFormatJSON,
			wantErr: false,
		},
		{
			name:    "json format - valid json null",
			data:    []byte(`null`),
			format:  configtypes.PublicationDataFormatJSON,
			wantErr: false,
		},
		{
			name:    "json format - invalid json",
			data:    []byte(`not valid json`),
			format:  configtypes.PublicationDataFormatJSON,
			wantErr: true,
			errMsg:  "data is not valid JSON",
		},
		{
			name:    "json format - empty data",
			data:    []byte{},
			format:  configtypes.PublicationDataFormatJSON,
			wantErr: true,
			errMsg:  "data is not valid JSON",
		},
		// Binary format tests
		{
			name:    "binary format - binary data allowed",
			data:    []byte{0x01, 0x02, 0x03},
			format:  configtypes.PublicationDataFormatBinary,
			wantErr: false,
		},
		{
			name:    "binary format - empty data allowed",
			data:    []byte{},
			format:  configtypes.PublicationDataFormatBinary,
			wantErr: false,
		},
		{
			name:    "binary format - text data allowed",
			data:    []byte("any text"),
			format:  configtypes.PublicationDataFormatBinary,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePublicationData(tt.data, tt.format)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
