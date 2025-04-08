//go:build integration

package consuming

import (
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

func testCommon(registry prometheus.Registerer) *consumerCommon {
	return &consumerCommon{
		name:    "test",
		nodeID:  uuid.New().String(),
		log:     log.With().Str("consumer", "test").Logger(),
		metrics: newCommonMetrics(registry),
	}
}
