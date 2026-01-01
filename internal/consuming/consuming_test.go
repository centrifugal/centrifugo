//go:build integration

package consuming

import (
	"os"
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/metrics"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

func TestMain(m *testing.M) {
	// Initialize metrics with a custom registry for tests to avoid conflicts
	registry := prometheus.NewRegistry()
	_ = metrics.Init(metrics.Config{
		Registerer: registry,
	})
	os.Exit(m.Run())
}

func testCommon(registry prometheus.Registerer) *consumerCommon {
	return &consumerCommon{
		name:   "test",
		nodeID: uuid.New().String(),
		log:    log.With().Str("consumer", "test").Logger(),
	}
}
