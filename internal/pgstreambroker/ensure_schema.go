package pgstreambroker

import (
	"context"
	"fmt"
)

// reconcileShardLock ensures the shard_lock table (for both variants) has
// exactly one row for each shard in [0, NumShards). Called on every
// EnsureSchema including the fast path so a NumShards change between runs
// is picked up. A stale shard_lock is load-bearing, not cosmetic: a missing
// row makes `PERFORM 1 FROM shard_lock WHERE shard_id = X FOR UPDATE`
// lock nothing, breaking per-shard publish serialization, which in turn
// lets stream IDs commit out of order and the outbox cursor skip rows.
func (e *PostgresStreamBroker) reconcileShardLock(ctx context.Context) error {
	for _, prefix := range []string{e.names.jsonbPrefix, e.names.binaryPrefix} {
		shardLock := prefix + "shard_lock"
		if _, err := e.pool.Exec(ctx, fmt.Sprintf(
			`INSERT INTO %s (shard_id) SELECT generate_series(0, $1 - 1) ON CONFLICT DO NOTHING`,
			shardLock), e.conf.NumShards); err != nil {
			return &SchemaError{
				Object: SchemaObject{Type: "table", Name: shardLock},
				Op:     "create",
				Err:    fmt.Errorf("populate shard_lock: %w", err),
			}
		}
		if _, err := e.pool.Exec(ctx, fmt.Sprintf(
			`DELETE FROM %s WHERE shard_id >= $1`,
			shardLock), e.conf.NumShards); err != nil {
			return &SchemaError{
				Object: SchemaObject{Type: "table", Name: shardLock},
				Op:     "create",
				Err:    fmt.Errorf("trim shard_lock: %w", err),
			}
		}
	}
	return nil
}

// EnsureSchema creates all required database objects idempotently.
//
// Flow:
//  1. Fast path: query schema_version. If current and the history table
//     exists AND is partitioned, reconcile shard_lock and return.
//  2. Render both variant schemas (jsonb + binary), execute the DDL part
//     and the function part separately under retry.
//  3. Populate shard_lock for both variants.
//  4. Verify the history table is partitioned for the active variant
//     (wrong-shape probe — fails loudly if a non-partitioned table exists
//     from a previous build).
//  5. Pre-create today's + lookahead partitions via the partitioner.
//  6. Update schema_version row.
func (e *PostgresStreamBroker) EnsureSchema(ctx context.Context) error {
	// Fast path: version already current and history table exists.
	var dbVersion int
	err := e.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT schema_version FROM %s WHERE id = 1`, e.names.schemaVersion),
	).Scan(&dbVersion)
	if err == nil && dbVersion == schemaVersion {
		if _, probeErr := e.pool.Exec(ctx, fmt.Sprintf(
			`SELECT 1 FROM %s LIMIT 0`, e.names.stream)); probeErr == nil {
			// Also verify partitioned shape — see step 4.
			if err := e.verifyPartitionedShape(ctx); err != nil {
				return err
			}
			// Reconcile shard_lock before returning: NumShards may have
			// changed since the last EnsureSchema call.
			if err := e.reconcileShardLock(ctx); err != nil {
				return err
			}
			// Ensure lookahead partitions exist (idempotent).
			return e.ensureInitialPartitions(ctx)
		}
	}
	if err != nil {
		dbVersion = 0
	}

	// Render and run DDL for both variants.
	jsonbSQL := renderSchema(e.names.jsonbPrefix, false)
	binarySQL := renderSchema(e.names.binaryPrefix, true)
	jsonbDDL, jsonbFuncs := splitSchemaSQL(jsonbSQL)
	binaryDDL, binaryFuncs := splitSchemaSQL(binarySQL)
	for _, sql := range []string{jsonbDDL, binaryDDL, jsonbFuncs, binaryFuncs} {
		if sql == "" {
			continue
		}
		if err := e.execSchemaWithRetry(ctx, sql); err != nil {
			return err
		}
	}

	// Populate/trim shard_lock for both variants.
	if err := e.reconcileShardLock(ctx); err != nil {
		return err
	}

	// Wrong-shape probe: ensure the history table is actually partitioned.
	if err := e.verifyPartitionedShape(ctx); err != nil {
		return err
	}

	// Pre-create today's + lookahead partitions for the active variant.
	if err := e.ensureInitialPartitions(ctx); err != nil {
		return err
	}

	// Run schema migrations (empty for v1).
	if dbVersion > 0 {
		for v := dbVersion + 1; v <= schemaVersion; v++ {
			if sql, ok := schemaMigrations[v]; ok {
				if err := e.execSchemaWithRetry(ctx, sql); err != nil {
					return err
				}
			}
		}
	}

	// Update version row.
	for _, prefix := range []string{e.names.jsonbPrefix, e.names.binaryPrefix} {
		if _, err := e.pool.Exec(ctx, fmt.Sprintf(
			`UPDATE %sschema_version SET schema_version = $1 WHERE id = 1`,
			prefix), schemaVersion); err != nil {
			e.logErrorMsg("schema version update", err)
		}
	}

	return nil
}

// verifyPartitionedShape probes pg_partitioned_table for the active variant's
// history table. If a non-partitioned table exists (from an earlier build of
// pgstreambroker), fail loudly with a clear "drop the legacy tables" message.
func (e *PostgresStreamBroker) verifyPartitionedShape(ctx context.Context) error {
	var isPartitioned bool
	err := e.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_partitioned_table
			WHERE partrelid = $1::regclass
		)
	`, e.names.stream).Scan(&isPartitioned)
	if err != nil {
		// Table doesn't exist yet (or other lookup error). Fall through —
		// the schema template's CREATE TABLE will run later.
		return nil
	}
	if !isPartitioned {
		return &SchemaError{
			Object: SchemaObject{Type: "table", Name: e.names.stream},
			Op:     "verify",
			Err: fmt.Errorf(
				"%s exists but is not partitioned. pgstreambroker schema requires "+
					"a partitioned table and this build does not include migration logic. "+
					"Drop the existing tables manually with: DROP TABLE IF EXISTS %s, %s, %s, %s, %s CASCADE",
				e.names.stream,
				e.names.stream, e.names.meta, e.names.idempotency, e.names.shardLock, e.names.schemaVersion,
			),
		}
	}
	return nil
}

// ensureInitialPartitions pre-creates today's and lookahead partitions
// via pgoutbox.Partitioner.EnsureLookaheadPartitions.
func (e *PostgresStreamBroker) ensureInitialPartitions(ctx context.Context) error {
	p := e.newPartitioner()
	if err := p.EnsureLookaheadPartitions(ctx); err != nil {
		return &SchemaError{
			Object: SchemaObject{Type: "table", Name: e.names.stream},
			Op:     "create",
			Err:    err,
		}
	}
	return nil
}
