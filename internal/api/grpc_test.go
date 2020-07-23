package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestGRPC_Authorize_Unauthenticated(t *testing.T) {
	err := authorize(context.Background(), []byte("apikey xxx"))
	require.Error(t, err)

	md := metadata.New(map[string]string{})
	ctx := metadata.NewIncomingContext(context.Background(), md)
	err = authorize(ctx, []byte("apikey xxx"))
	require.Error(t, err)

	md = metadata.New(map[string]string{
		"authorization": "apikey yyy",
	})
	ctx = metadata.NewIncomingContext(context.Background(), md)
	err = authorize(ctx, []byte("apikey xxx"))
	require.Error(t, err)
}

func TestGRPC_Authorize_OK(t *testing.T) {
	md := metadata.New(map[string]string{
		"authorization": "apikey xxx",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)
	err := authorize(ctx, []byte("apikey xxx"))
	require.NoError(t, err)
}
