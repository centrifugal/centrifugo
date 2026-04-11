package pgstreambroker

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/pgoutbox"
)

// newPartitioner constructs a pgoutbox.Partitioner configured for the
// stream broker's history table. Used at schema-init time (one-shot
// EnsureLookaheadPartitions) and by runPartitionWorker (periodic
// maintenance via pgoutbox.Partitioner.Run).
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

// runPartitionWorker manages partition lookahead creation and (if
// PartitionRetentionDays > 0) retention drops. Thin wrapper around
// pgoutbox.Partitioner.Run. Runs unconditionally — the schema is always
// partitioned and the lookahead pass is required for writes to succeed
// at the day rollover.
func (e *PostgresStreamBroker) runPartitionWorker() {
	p := e.newPartitioner()
	p.Run(e.cancelCtx, e.closeCh)
}

// runCleanupWorker runs the cleanup pass on CleanupInterval ticks. The
// simplified cleanup design only touches the small support tables (meta
// and idempotency) — history rows are cleaned up by partition retention.
//
// When FineGrainedHistoryCleanup is enabled, additionally runs the chunked
// per-channel history TTL DELETE pass.
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
		}
	}
}

// cleanupSupportTables deletes expired meta and idempotency rows.
// Single-statement DELETEs — no chunking needed since both tables are small.
func (e *PostgresStreamBroker) cleanupSupportTables(ctx context.Context) {
	if _, err := e.pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s WHERE meta_expires_at IS NOT NULL AND meta_expires_at < NOW()`,
		e.names.meta,
	)); err != nil {
		e.logErrorMsg("error cleaning up expired meta", err)
	}
	if _, err := e.pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s WHERE expires_at < NOW()`,
		e.names.idempotency,
	)); err != nil {
		e.logErrorMsg("error cleaning up expired idempotency", err)
	}
}

// cleanupHistoryFineGrained runs the chunked DELETE pass for per-channel
// HistoryTTL precision. Only invoked when FineGrainedHistoryCleanup is true.
// The JOIN against meta plus the per-channel cutoff filter is more expensive
// than partition retention, so users only enable this when storage matters
// more than CPU.
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
