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

func TestDispatcherErrorsAfterNodeShutdown(t *testing.T) {
	n := nodeWithMemoryEngine()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	dispatcher := NewDispatcher(NewConsumingHandler(n, NewExecutor(n, cfgContainer, nil, ExecutorConfig{
		Protocol: "consumer",
	}), ConsumingHandlerConfig{}))

	const payload = `{"channel": "test", "data": {}}`

	// While node is running message must be dispatched successfully.
	require.NoError(t, dispatcher.DispatchCommand(context.Background(), "publish", []byte(payload)))

	require.NoError(t, n.Shutdown(context.Background()))

	// Memory broker keeps accepting publications even after Node shutdown – without
	// an explicit check a consumer would acknowledge a message which was never and
	// will never be delivered.
	err = dispatcher.DispatchCommand(context.Background(), "publish", []byte(payload))
	require.ErrorIs(t, err, ErrShutdown)
	// Consumers must stop processing instead of retrying until shutdown timeout.
	require.ErrorIs(t, err, context.Canceled)

	err = dispatcher.DispatchPublication(context.Background(), []string{"test"}, ConsumedPublication{
		Data: []byte(`{}`),
	})
	require.ErrorIs(t, err, ErrShutdown)
}
