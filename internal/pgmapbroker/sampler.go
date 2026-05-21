package pgmapbroker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/metrics"
)

// mapMetricsSampler periodically samples PG-specific gauge metrics: outbox
// cursor lag per shard and partition count on the stream table.
type mapMetricsSampler struct {
	broker        *PostgresMapBroker
	mu            sync.Mutex
	cursorByShard map[int]int64
}

func newMapMetricsSampler(b *PostgresMapBroker) *mapMetricsSampler {
	return &mapMetricsSampler{
		broker:        b,
		cursorByShard: make(map[int]int64),
	}
}

func (s *mapMetricsSampler) storeCursor(shard int, cursor int64) {
	s.mu.Lock()
	s.cursorByShard[shard] = cursor
	s.mu.Unlock()
}

func (s *mapMetricsSampler) sample(ctx context.Context) {
	if metrics.PGBrokerPartitions == nil {
		return
	}
	brokerName := s.broker.conf.Name

	// Sample partition count on the stream table.
	var partitionCount int
	err := s.broker.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_inherits i
		 JOIN pg_class c ON c.oid = i.inhparent
		 WHERE c.relname = $1
	`, s.broker.names.stream).Scan(&partitionCount)
	if err == nil {
		metrics.PGBrokerPartitions.WithLabelValues(brokerName).Set(float64(partitionCount))
	}

	// Sample cursor lag per shard.
	s.mu.Lock()
	cursors := make(map[int]int64, len(s.cursorByShard))
	for k, v := range s.cursorByShard {
		cursors[k] = v
	}
	s.mu.Unlock()
	for shard, cursor := range cursors {
		var createdAt time.Time
		err := s.broker.pool.QueryRow(ctx, fmt.Sprintf(
			`SELECT created_at FROM %s WHERE id = $1 LIMIT 1`,
			s.broker.names.stream,
		), cursor).Scan(&createdAt)
		if err != nil {
			metrics.PGBrokerOutboxCursorLagSeconds.WithLabelValues(brokerName, fmt.Sprintf("%d", shard)).Set(0)
			continue
		}
		lag := time.Since(createdAt).Seconds()
		if lag < 0 {
			lag = 0
		}
		metrics.PGBrokerOutboxCursorLagSeconds.WithLabelValues(brokerName, fmt.Sprintf("%d", shard)).Set(lag)
	}
}
