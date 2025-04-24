package api

import (
	"context"
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/stretchr/testify/require"
)

func TestNewConsumingHandler(t *testing.T) {
	n := nodeWithMemoryEngine()
	defer func() { _ = n.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	handler := NewConsumingHandler(n, NewExecutor(n, cfgContainer, nil, ExecutorConfig{
		Protocol:         "consumer",
		UseOpenTelemetry: false,
	}), ConsumingHandlerConfig{})

	dispatcher := NewDispatcher(handler)

	// Bad request must be just logged but no errors other than Internal Error should be returned from Dispatch.
	err = dispatcher.DispatchCommand(context.Background(), "publish", []byte(`{}`))
	require.NoError(t, err)
}
