package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/centrifugal/centrifugo/v6/internal/apiproto"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
)

func captureLog(t *testing.T) *bytes.Buffer {
	var out bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(zerolog.SyncWriter(&out))
	t.Cleanup(func() { log.Logger = prev })
	return &out
}

func newLogTestExecutor(t *testing.T) (*centrifuge.Node, *Executor) {
	node := nodeWithMemoryEngine()
	t.Cleanup(func() { _ = node.Shutdown(context.Background()) })
	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.HistorySize = 1
	cfg.Channel.WithoutNamespace.HistoryTTL = configtypes.Duration(time.Minute)
	container, err := config.NewContainer(cfg)
	require.NoError(t, err)
	return node, NewExecutor(node, container, &testSurveyCaller{}, ExecutorConfig{Protocol: "test"})
}

func TestLogAPIError(t *testing.T) {
	for _, tt := range []struct {
		err   error
		level string
	}{
		{centrifuge.ErrorBadRequest, "warn"},
		{fmt.Errorf("wrapped: %w", centrifuge.ErrorBadRequest), "warn"},
		{centrifuge.ErrorUnknownChannel, "warn"},
		{centrifuge.ErrorUnrecoverablePosition, "warn"},
		{ErrorNotFound, "warn"},
		{ErrorConflict, "warn"},
		{errors.New("broker is down"), "error"},
		{centrifuge.ErrorInternal, "error"},
		{centrifuge.ErrorNotAvailable, "error"},
		{centrifuge.ErrorTooManyRequests, "error"},
	} {
		t.Run(tt.err.Error(), func(t *testing.T) {
			out := captureLog(t)
			logAPIError(tt.err).Msg("failed")
			require.Contains(t, out.String(), `"level":"`+tt.level+`"`)
		})
	}
}

func TestAPIBadRequestLogLevel(t *testing.T) {
	node, executor := newLogTestExecutor(t)
	ctx := context.Background()
	for name, call := range map[string]func(){
		"publish without channel":   func() { executor.Publish(ctx, &PublishRequest{}) },
		"broadcast without channel": func() { executor.Broadcast(ctx, &BroadcastRequest{}) },
		"subscribe without channel": func() { executor.Subscribe(ctx, &SubscribeRequest{}) },
		"rpc without method":        func() { executor.RPC(ctx, &RPCRequest{}) },
		"history wrong epoch": func() {
			executor.History(ctx, &HistoryRequest{Channel: "test", Since: &StreamPosition{Epoch: "other"}})
		},
		"history reverse since zero": func() {
			executor.History(ctx, &HistoryRequest{Channel: "test", Reverse: true, Since: &StreamPosition{}})
		},
		"malformed body": func() {
			h := NewHandler(node, executor, Config{})
			h.handlePublish(httptest.NewRecorder(), httptest.NewRequest("POST", "/publish", strings.NewReader("{")))
		},
	} {
		t.Run(name, func(t *testing.T) {
			out := captureLog(t)
			call()
			require.Contains(t, out.String(), `"level":"warn"`)
			require.NotContains(t, out.String(), `"level":"error"`)
		})
	}
}

// A refused message from a consumer is lost, so it must stay at error level.
func TestConsumingBroadcastSkippedLogLevel(t *testing.T) {
	node, executor := newLogTestExecutor(t)
	d := NewDispatcher(NewConsumingHandler(node, executor, ConsumingHandlerConfig{}))
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"publication": func() error {
			return d.DispatchPublication(ctx, []string{"test", "unknown:test"}, ConsumedPublication{Data: []byte("{}")})
		},
		"command": func() error {
			return d.DispatchCommand(ctx, "broadcast", []byte(`{"channels":["test","unknown:test"],"data":{}}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			out := captureLog(t)
			require.NoError(t, call())
			require.Equal(t, 1, strings.Count(out.String(), `"level":"error"`), out.String())
			require.Contains(t, out.String(), "skip message")
		})
	}
}
