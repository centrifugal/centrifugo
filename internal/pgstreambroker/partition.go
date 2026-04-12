package pgstreambroker

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/metrics"
	"github.com/centrifugal/centrifugo/v6/internal/pgoutbox"
)

// newPartitioner constructs a pgoutbox.Partitioner configured for the
// stream broker's history table.
func (e *PostgresStreamBroker) newPartitioner() *pgoutbox.Partitioner {
	return &pgoutbox.Partitioner{
		Pool:            e.pool,
		ParentTable:     e.names.history,
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
			if e.conf.FineGrainedHistoryCleanup {
				e.cleanupHistoryFineGrained(e.cancelCtx)
			}
			if e.sampler != nil {
				e.sampler.sample(e.cancelCtx)
			}
		}
	}
}

func addCleanupRows(broker, pass string, n int64) {
	if metrics.PGBrokerCleanupRowsDeletedTotal != nil {
		metrics.PGBrokerCleanupRowsDeletedTotal.WithLabelValues(broker, pass).Add(float64(n))
	}
}

// cleanupSupportTables deletes expired meta and idempotency rows.
func (e *PostgresStreamBroker) cleanupSupportTables(ctx context.Context) {
	if res, err := e.pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s WHERE meta_expires_at IS NOT NULL AND meta_expires_at < NOW()`,
		e.names.meta,
	)); err != nil {
		e.logErrorMsg("error cleaning up expired meta", err)
	} else {
		addCleanupRows(e.conf.Name, "meta", res.RowsAffected())
	}
	if res, err := e.pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s WHERE expires_at < NOW()`,
		e.names.idempotency,
	)); err != nil {
		e.logErrorMsg("error cleaning up expired idempotency", err)
	} else {
		addCleanupRows(e.conf.Name, "idempotency", res.RowsAffected())
	}
}

// cleanupHistoryFineGrained runs the chunked DELETE pass for per-channel
// HistoryTTL precision. Only invoked when FineGrainedHistoryCleanup is true.
func (e *PostgresStreamBroker) cleanupHistoryFineGrained(ctx context.Context) {
	query := fmt.Sprintf(`
		WITH victims AS (
			SELECT h.id, h.created_at
			  FROM %s h
			  JOIN %s m ON m.channel = h.channel
			 WHERE h.kind = 0
			   AND m.history_ttl IS NOT NULL
			   AND h.created_at < NOW() - m.history_ttl
			 LIMIT $1
		)
		DELETE FROM %s h
		 USING victims v
		 WHERE h.id = v.id AND h.created_at = v.created_at
	`, e.names.history, e.names.meta, e.names.history)

	for {
		res, err := e.pool.Exec(ctx, query, e.conf.CleanupBatchSize)
		if err != nil {
			e.logErrorMsg("error in fine-grained history cleanup", err)
			return
		}
		addCleanupRows(e.conf.Name, "history_ttl_fine_grained", res.RowsAffected())
		if res.RowsAffected() < int64(e.conf.CleanupBatchSize) {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-e.closeCh:
			return
		case <-time.After(e.conf.CleanupChunkPause):
		}
	}
}
