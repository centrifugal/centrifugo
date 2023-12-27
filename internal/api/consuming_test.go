package api

import (
	"context"
	"testing"

	"github.com/centrifugal/centrifugo/v5/internal/rule"

	"github.com/stretchr/testify/require"
)

func TestNewConsumingHandler(t *testing.T) {
	n := nodeWithMemoryEngine()
	defer func() { _ = n.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.ClientInsecure = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)

	handler := NewConsumingHandler(n, NewExecutor(n, ruleContainer, nil, ExecutorConfig{
		Protocol:         "consumer",
		UseOpenTelemetry: false,
	}), ConsumingHandlerConfig{})

	// Bad request must be just logged but no errors other than Internal Error should be returned from Dispatch.
	err = handler.Dispatch(context.Background(), "publish", []byte(`{}`))
	require.NoError(t, err)
}
