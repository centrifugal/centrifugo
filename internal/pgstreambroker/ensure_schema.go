package pgstreambroker

import (
	"context"
	"fmt"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/pgschema"
)

// reconcileShardLock ensures the shard_lock table (for both variants) has
// exactly one row for each shard in [0, NumShards). Called on every
// EnsureSchema including the fast path so a NumShards change between runs
// is picked up. A stale shard_lock is load-bearing, not cosmetic: a missing
// row makes `PERFORM 1 FROM shard_lock WHERE shard_id = X FOR UPDATE`
// lock nothing, breaking per-shard publish serialization, which in turn
// lets stream IDs commit out of order and the outbox cursor skip rows.
// bothVariantsPresent reports whether the primary table exists for both the
// jsonb and binary variants, so EnsureSchema's fast path doesn't trust a partial
// install where only one variant was created.
func (e *PostgresStreamBroker) bothVariantsPresent(ctx context.Context) bool {
	for _, prefix := range []string{e.names.jsonbPrefix, e.names.binaryPrefix} {
		table := strings.TrimRight(prefix, "_")
		if _, err := e.pool.Exec(ctx, fmt.Sprintf(`SELECT 1 FROM %s LIMIT 0`, table)); err != nil {
			return false
		}
	}
	return true
}

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

// versionTables returns the per-variant schema_version table names — used as
// the targets for every UPDATE schema_version performed during the schema
// lifecycle (specifically the final SetSchemaVersion after DDL).
func (e *PostgresStreamBroker) versionTables() []string {
	return []string{
		e.names.jsonbPrefix + "schema_version",
		e.names.binaryPrefix + "schema_version",
	}
}

// migrationVariants renders `template` once per variant (jsonb + binary) and
// pairs each rendered SQL with its variant's schema_version table.
func (e *PostgresStreamBroker) migrationVariants(template string) []pgschema.MigrationVariant {
	return []pgschema.MigrationVariant{
		{
			SQL:          renderSchemaTemplate(template, e.names.jsonbPrefix, false),
			VersionTable: e.names.jsonbPrefix + "schema_version",
		},
		{
			SQL:          renderSchemaTemplate(template, e.names.binaryPrefix, true),
			VersionTable: e.names.binaryPrefix + "schema_version",
		},
	}
}

// runMigrationsUnderLock — see pgmapbroker.runMigrationsUnderLock; identical
// shape and rationale, scoped to this broker's prefix tables.
func (e *PostgresStreamBroker) runMigrationsUnderLock(ctx context.Context, label string) error {
	release, err := pgschema.AcquireMigrationLock(ctx, e.pool, label)
	if err != nil {
		return err
	}
	defer release()

	dbVersion, isFresh, err := pgschema.ReadSchemaVersion(ctx, e.pool, e.names.schemaVersion)
	if err != nil {
		return err
	}
	if isFresh {
		return nil
	}
	if err := pgschema.CheckDowngrade(label, dbVersion, schemaVersion); err != nil {
		return err
	}
	for v := dbVersion + 1; v <= schemaVersion; v++ {
		sql, ok := schemaMigrations[v]
		if !ok {
			return fmt.Errorf("%s: missing schemaMigrations[%d] at runtime (schemaVersion=%d)", label, v, schemaVersion)
		}
		if err := pgschema.ApplyMigrationInTx(ctx, e.pool, label, v, e.migrationVariants(sql)); err != nil {
			return err
		}
	}
	return nil
}

// EnsureSchema creates all required database objects idempotently.
//
// Flow:
//  1. Read schema_version with error discrimination (transient errors must
//     propagate — see pgschema.ReadSchemaVersion).
//  2. Fast path: version matches AND probe ok AND partitioned shape verified
//     → reconcile shard_lock + lookahead partitions and return.
//  3. Reject downgrade.
//  4. Run pending migrations BEFORE DDL, each in its own transaction with
//     atomic schema_version bump.
//  5. Render and run DDL for both variants (CREATE OR REPLACE picks up the
//     latest function bodies; CREATE TABLE IF NOT EXISTS is a no-op when
//     migrations have already brought the shape current).
//  6. Reconcile shard_lock; verify partitioned shape; pre-create partitions.
//  7. Final schema_version write — fatal on failure.
func (e *PostgresStreamBroker) EnsureSchema(ctx context.Context) error {
	const label = "pgstreambroker"

	dbVersion, isFresh, err := pgschema.ReadSchemaVersion(ctx, e.pool, e.names.schemaVersion)
	if err != nil {
		return err
	}

	// Fast path: version current, both variants' history tables exist, partition
	// shape ok. Probe BOTH variants (not just the active one): the two variants
	// are created by separate Exec calls, so a partial fresh install can commit
	// one and fail the other transiently. Trusting only the active variant would
	// take the fast path and then fail in reconcileShardLock (which touches both)
	// on the missing tables, before reaching the idempotent DDL that recreates
	// them — wedging the broker on every restart. If a variant is missing, fall
	// through and re-apply DDL to self-heal.
	if !isFresh && dbVersion == schemaVersion {
		if e.bothVariantsPresent(ctx) {
			if err := e.verifyPartitionedShape(ctx); err != nil {
				return err
			}
			if err := e.reconcileShardLock(ctx); err != nil {
				return err
			}
			return e.ensureInitialPartitions(ctx)
		}
	}

	if !isFresh {
		if err := pgschema.CheckDowngrade(label, dbVersion, schemaVersion); err != nil {
			return err
		}
	}

	// Migrations run BEFORE DDL, under a Postgres advisory lock so concurrent
	// rolling-deploy upgrades can't run the same migration chain twice.
	// See pgmapbroker.EnsureSchema for the full rationale.
	if !isFresh {
		if err := e.runMigrationsUnderLock(ctx, label); err != nil {
			return err
		}
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

	if err := e.reconcileShardLock(ctx); err != nil {
		return err
	}
	if err := e.verifyPartitionedShape(ctx); err != nil {
		return err
	}
	if err := e.ensureInitialPartitions(ctx); err != nil {
		return err
	}

	// Final schema_version write — see pgmapbroker for the rationale on
	// "fatal on failure".
	return pgschema.SetSchemaVersion(ctx, e.pool, label, schemaVersion, e.versionTables())
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
