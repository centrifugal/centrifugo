package proxyproto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestMarshalJSON(t *testing.T) {
	msg := &RPCRequest{
		Data: []byte(`{"input": "привет"}`),
	}
	_, err := json.Marshal(msg)
	require.NoError(t, err)
}

func TestMarshalProtobuf(t *testing.T) {
	msg := &RPCRequest{
		Data: []byte(`{"input": "привет"}`),
	}
	_, err := proto.Marshal(msg)
	require.NoError(t, err)
}
