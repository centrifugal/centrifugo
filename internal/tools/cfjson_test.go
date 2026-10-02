package tools

import (
	"testing"

	"github.com/centrifugal/protocol"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalJSONPtr(t *testing.T) {
	var req *protocol.ConnectRequest
	require.NoError(t, UnmarshalJSONPtr([]byte(` null `), &req))
	require.Nil(t, req)

	require.NoError(t, UnmarshalJSONPtr([]byte(`{"token":"t"}`), &req))
	require.Equal(t, "t", req.Token)

	require.NoError(t, UnmarshalJSONPtr([]byte(`null`), &req))
	require.Nil(t, req)

	require.Error(t, UnmarshalJSONPtr([]byte(`null x`), &req))
	require.Error(t, UnmarshalJSONPtr([]byte(`nul`), &req))
	require.Error(t, UnmarshalJSONPtr([]byte(`nullx`), &req))
	require.Error(t, UnmarshalJSONPtr([]byte(`{"token":1}`), &req))
	require.Error(t, UnmarshalJSONPtr([]byte(``), &req))
}

// Decoded strings are copies: the input may be reused or changed afterwards
// (transports read connect requests into buffers they reuse).
func TestUnmarshalJSONPtrCopies(t *testing.T) {
	data := []byte(`{"token":"secret","name":"js","version":"1.0"}`)
	var req *protocol.ConnectRequest
	require.NoError(t, UnmarshalJSONPtr(data, &req))
	for i := range data {
		data[i] = 'x'
	}
	require.Equal(t, "secret", req.Token)
	require.Equal(t, "js", req.Name)
	require.Equal(t, "1.0", req.Version)
}
