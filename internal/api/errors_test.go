package api

import (
	"context"
	"errors"
	"fmt"
	"testing"

	. "github.com/centrifugal/centrifugo/v6/internal/apiproto"
	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

func TestToAPIErr(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want *Error
	}{
		{"broker failure", errors.New("connection refused"), ErrorInternal},
		{"API error", ErrorConflict, ErrorConflict},
		{"unknown channel", centrifuge.ErrorUnknownChannel, ErrorUnknownChannel},
		{"bad request, wrapped", fmt.Errorf("%w: channel name is not supported", centrifuge.ErrorBadRequest), ErrorBadRequest},
		{"not available", centrifuge.ErrorNotAvailable, ErrorNotAvailable},
		{"too many requests", centrifuge.ErrorTooManyRequests, ErrorTooManyRequests},
		{"unrecoverable position", centrifuge.ErrorUnrecoverablePosition, ErrorUnrecoverablePosition},
		// 113 means another thing in the client protocol than in the API.
		{"concurrent pagination, temporary", centrifuge.ErrorConcurrentPagination, ErrorInternal},
		{"no API equivalent, temporary", &centrifuge.Error{Code: 4000, Temporary: true}, ErrorInternal},
		{"no API equivalent, permanent", centrifuge.ErrorLimitExceeded, ErrorBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, toAPIErr(tc.err))
		})
	}
}

// failingBroker fails every publication with err. It embeds the Broker
// interface, not a broker, so that it has no methods but Broker's: a broker
// which publishes in batches would take publications past Publish.
type failingBroker struct {
	centrifuge.Broker
	err error
}

func (b *failingBroker) Publish(string, []byte, centrifuge.PublishOptions) (centrifuge.PublishResult, error) {
	return centrifuge.PublishResult{}, b.err
}

// A publication the Node refuses is reported as such, not as a temporary
// internal error, and consumers skip it instead of retrying it.
func TestNodeErrorsReachAPI(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		want      *Error
		consumeOK bool
	}{
		{"refused", fmt.Errorf("%w: channel name is not supported", centrifuge.ErrorBadRequest), ErrorBadRequest, true},
		{"broker failure", errors.New("connection refused"), ErrorInternal, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node, err := centrifuge.New(centrifuge.Config{})
			require.NoError(t, err)
			memory, err := centrifuge.NewMemoryBroker(node, centrifuge.MemoryBrokerConfig{})
			require.NoError(t, err)
			node.SetBroker(&failingBroker{Broker: memory, err: tc.err})
			require.NoError(t, node.Run())
			defer func() { _ = node.Shutdown(context.Background()) }()

			cfgContainer, err := config.NewContainer(config.DefaultConfig())
			require.NoError(t, err)
			executor := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test"})

			resp := executor.Publish(context.Background(), &PublishRequest{Channel: "test", Data: []byte("{}")})
			require.Equal(t, tc.want, resp.Error)

			bResp := executor.Broadcast(context.Background(), &BroadcastRequest{Channels: []string{"test"}, Data: []byte("{}")})
			require.Nil(t, bResp.Error)
			require.Equal(t, tc.want, bResp.Result.Responses[0].Error)

			dispatcher := NewDispatcher(NewConsumingHandler(node, executor, ConsumingHandlerConfig{}))
			err = dispatcher.DispatchCommand(context.Background(), "publish", []byte(`{"channel":"test","data":{}}`))
			if tc.consumeOK {
				require.NoError(t, err, "a refused publication must be skipped, not retried")
			} else {
				require.Error(t, err, "a temporary error must be retried")
			}
		})
	}
}
