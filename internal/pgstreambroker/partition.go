package pgstreambroker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/metrics"
	"github.com/centrifugal/centrifugo/v6/internal/pgoutbox"
)

// isShutdownErr reports whether err is a normal-shutdown signal — context
// cancellation surfacing because the broker is being closed mid-tick. Those
// aren't real failures and shouldn't be logged at error level, since they
// would otherwise pollute CI logs and mask genuine cleanup errors.
func isShutdownErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// newPartitioner constructs a pgoutbox.Partitioner configured for the
// stream broker's history table.
func (e *PostgresStreamBroker) newPartitioner() *pgoutbox.Partitioner {
	return &pgoutbox.Partitioner{
		Pool:            e.pool,
		ParentTable:     e.names.stream,
		CleanupInterval: e.conf.CleanupInterval,
		LookaheadDays:   e.conf.PartitionLookaheadDays,
		RetentionDays:   e.conf.PartitionRetentionDays,
		ErrorFn:         e.logErrorMsg,
	}
}

// runPartitionWorker manages partition lookahead creation and retention drops.
func (e *PostgresStreamBroker) runPartitionWorker() {
	p := e.newPartitioner()
	p.Run(e.cancelCtx, e.closeCh)
}

// runCleanupWorker runs the cleanup pass on CleanupInterval ticks.
func (e *PostgresStreamBroker) runCleanupWorker() {
	ticker := time.NewTicker(e.conf.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-e.cancelCtx.Done():
			return
		case <-e.closeCh:
			return
		case <-ticker.C:
			e.cleanupSupportTables(e.cancelCtx)
			if e.sampler != nil {
				e.sampler.sample(e.cancelCtx)
			}
		}
	}
}

func addCleanupRows(broker, pass string, n int64) {
	if metrics.BrokerPostgresCleanupRemovedTotal != nil {
		metrics.BrokerPostgresCleanupRemovedTotal.WithLabelValues(broker, pass).Add(float64(n))
	}
}

// cleanupSupportTables deletes expired meta and idempotency rows in bounded
// batches to avoid long-running DELETE locks on busy tables. Each batch is
// capped at cleanupBatchSize rows; passes repeat with a cleanupChunkPause gap
// until fewer than a full batch is deleted. Context cancellation (broker
// shutting down mid-tick) is treated as normal and not logged.
func (e *PostgresStreamBroker) cleanupSupportTables(ctx context.Context) {
	e.cleanupExpiredBatched(ctx, "meta", fmt.Sprintf(
		`DELETE FROM %[1]s WHERE channel IN (
			SELECT channel FROM %[1]s
			WHERE expires_at IS NOT NULL AND expires_at < NOW()
			LIMIT $1
		)`, e.names.meta))
	e.cleanupExpiredBatched(ctx, "idempotency", fmt.Sprintf(
		`DELETE FROM %[1]s WHERE (channel, idempotency_key) IN (
			SELECT channel, idempotency_key FROM %[1]s
			WHERE expires_at < NOW()
			LIMIT $1
		)`, e.names.idempotency))
}

const (
	cleanupBatchSize  = 1000
	cleanupChunkPause = 100 * time.Millisecond
)

// cleanupExpiredBatched runs a chunked DELETE loop for the given table.
// Stops when a batch deletes fewer rows than cleanupBatchSize or ctx is done.
func (e *PostgresStreamBroker) cleanupExpiredBatched(ctx context.Context, passName, query string) {
	for {
		res, err := e.pool.Exec(ctx, query, cleanupBatchSize)
		if err != nil {
			if !isShutdownErr(err) {
				e.logErrorMsg("error cleaning up expired "+passName, err)
			}
			return
		}
		addCleanupRows(e.conf.Name, passName, res.RowsAffected())
		if res.RowsAffected() < cleanupBatchSize {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-e.closeCh:
			return
		case <-time.After(cleanupChunkPause):
		}
	}
}
