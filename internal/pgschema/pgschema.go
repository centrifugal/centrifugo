// Package pgschema contains the shared schema-versioning primitives used by
// every PostgreSQL-backed broker / controller in this project. Each consumer
// stores its own integer schemaVersion plus a map[int]string of upgrade
// migrations and wires them together in its own EnsureSchema; the safety
// invariants (gap-free migrations, error-discriminated reads, transactional
// upgrades, downgrade rejection) all live here so they can't drift apart.
package pgschema

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// querier is the minimal subset of *pgxpool.Pool / pgx.Tx that the
// schema-management helpers need. Accepting an interface keeps the package
// free of a hard pgxpool dependency and makes it usable from inside an
// outer transaction (e.g. a test fixture that wraps everything in a txn).
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// txBeginner is what ApplyMigrationInTx needs from the connection source —
// the ability to start a new transaction. Pools satisfy this directly.
type txBeginner interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

// ValidateMigrationMap panics if migrations doesn't contain exactly one entry
// for every version in [2..target] (with nothing outside that range). Call
// this from a package-level init() in every broker that ships migrations.
//
// Rationale: bumping schemaVersion without registering the matching migration
// would silently skip it for existing installs — schema_version would jump
// forward via the final UPDATE while the DB shape stays at the old version.
// Panic at process start rather than corrupt at runtime.
func ValidateMigrationMap(label string, target int, migrations map[int]string) {
	for v := 2; v <= target; v++ {
		if _, ok := migrations[v]; !ok {
			panic(fmt.Sprintf("%s: missing schemaMigrations[%d] (schemaVersion=%d) — every version 2..schemaVersion must have a migration registered", label, v, target))
		}
	}
	for v := range migrations {
		if v < 2 || v > target {
			panic(fmt.Sprintf("%s: schemaMigrations[%d] is outside the valid range [2..%d]", label, v, target))
		}
	}
}

// ReadSchemaVersion reads the schema_version row from `table` and discriminates
// the error class. Returns:
//
//   - (version, false, nil) if the row was read successfully
//   - (0, true, nil)        if the table is missing (Postgres 42P01) or the
//     row is missing (pgx.ErrNoRows). The caller should run the fresh-install
//     path: DDL creates everything at the latest baseline, then SetSchemaVersion
//     bumps from 1 to the current schemaVersion.
//   - (0, false, err)       for any other error (transient connection failure,
//     permission denied, timeout, column missing, …). The caller MUST propagate.
//
// Why: collapsing every error into "treat as fresh" lets a transient SELECT
// failure silently bypass migrations and force schema_version forward via the
// final UPDATE — leaving the DB at the old shape while the row claims the new
// version (silent corruption).
func ReadSchemaVersion(ctx context.Context, q querier, table string) (int, bool, error) {
	var v int
	err := q.QueryRow(ctx, fmt.Sprintf(`SELECT schema_version FROM %s WHERE id = 1`, table)).Scan(&v)
	if err == nil {
		return v, false, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, true, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		return 0, true, nil
	}
	return 0, false, fmt.Errorf("read schema_version from %s: %w", table, err)
}

// CheckDowngrade returns a clear error when the DB has been touched by a
// newer binary than this one. Silently rewriting schema_version backward (the
// pre-pgschema behaviour) would leave the DB structurally newer than the
// running code expects.
func CheckDowngrade(label string, dbVersion, supportedVersion int) error {
	if dbVersion > supportedVersion {
		return fmt.Errorf("%s: DB schema_version is %d but this binary supports only up to %d — downgrade not supported, run a binary >= %d or restore an older DB snapshot", label, dbVersion, supportedVersion, dbVersion)
	}
	return nil
}

// MigrationVariant pairs a fully-rendered migration SQL with the version
// tracking table it should bump. ApplyMigrationInTx runs every variant in
// the SAME transaction — either all variants commit or all roll back.
//
// The "variant" concept exists because brokers can store both a JSONB-typed
// and a BYTEA-typed copy of the schema under different table prefixes; each
// variant's SQL is the same migration template rendered for one prefix. A
// broker with no variants (e.g. the postgres controller) passes a one-element
// slice.
type MigrationVariant struct {
	// SQL is the migration body, with all template placeholders already
	// substituted by the caller (e.g. via the broker's own render helper).
	// pgschema is intentionally agnostic about substitution rules so each
	// broker can use its own renderer (pgstreambroker, for instance, also
	// substitutes __STREAM_TABLE__).
	SQL string
	// VersionTable is the fully-qualified `<prefix>schema_version` table
	// whose id=1 row this variant bumps. A row with no matching id=1 returns
	// an error rather than silently no-op'ing.
	VersionTable string
}

// ApplyMigrationInTx applies one migration step. For each variant in the
// slice:
//   - The variant's SQL is executed.
//   - The variant's VersionTable id=1 row is updated to `version`.
//
// All variants run in one transaction; a failure on any rolls back every
// effect, including any partial version bumps. The transactional wrap makes
// retries safe even when migration SQL is not perfectly idempotent — the
// previous attempt has fully rolled back.
//
// The body is retried up to migrationRetryAttempts times on the same
// transient codes execSchemaWithRetry retries (40P01 deadlock_detected,
// XX000 "tuple concurrently updated" — surfaces from concurrent
// CREATE OR REPLACE in a rolling deploy). ctx cancellation aborts the
// retry loop immediately.
func ApplyMigrationInTx(ctx context.Context, p txBeginner, label string, version int, variants []MigrationVariant) error {
	if len(variants) == 0 {
		return fmt.Errorf("%s: ApplyMigrationInTx v%d called with no variants", label, version)
	}
	var lastErr error
	for attempt := 0; attempt < migrationRetryAttempts; attempt++ {
		err := applyMigrationOnce(ctx, p, label, version, variants)
		if err == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "XX000") && attempt < migrationRetryAttempts-1 {
			lastErr = err
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(migrationRetryBackoff):
			}
			continue
		}
		return err
	}
	return lastErr
}

const (
	migrationRetryAttempts = 3
	migrationRetryBackoff  = 200 * time.Millisecond
)

func applyMigrationOnce(ctx context.Context, p txBeginner, label string, version int, variants []MigrationVariant) error {
	tx, err := p.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("%s: begin tx for migration v%d: %w", label, version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, variant := range variants {
		if _, err := tx.Exec(ctx, variant.SQL); err != nil {
			return fmt.Errorf("%s: apply migration v%d (target %s): %w", label, version, variant.VersionTable, err)
		}
		ct, err := tx.Exec(ctx, fmt.Sprintf(
			`UPDATE %s SET schema_version = $1 WHERE id = 1`, variant.VersionTable), version)
		if err != nil {
			return fmt.Errorf("%s: bump schema_version to v%d (%s): %w", label, version, variant.VersionTable, err)
		}
		if ct.RowsAffected() == 0 {
			return fmt.Errorf("%s: %s has no row id=1 — version tracking is corrupt, manually INSERT (1, %d) and retry", label, variant.VersionTable, version-1)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%s: commit migration v%d: %w", label, version, err)
	}
	return nil
}

// AcquireMigrationLock takes a session-scoped Postgres advisory lock keyed on
// `label`. Callers MUST defer the returned release function. While the lock
// is held, every concurrent AcquireMigrationLock call against the same
// Postgres instance and same label blocks — serialising the migration loop
// across nodes during a rolling deploy.
//
// Implementation: a dedicated connection is checked out of the pool and held
// for the lifetime of the lock. Migrations themselves run on different pool
// connections; advisory locks are visible globally on the server, so the
// lock-holding connection's identity is sufficient to serialise others.
//
// The release function explicitly executes pg_advisory_unlock BEFORE returning
// the connection to the pool, so the next pool user does not inherit the lock.
// If unlock fails (e.g. the connection has died), Postgres releases the lock
// automatically on session end — no lock is ever stranded across process
// crashes.
//
// The lock ID is derived from a FNV-64 hash of "pgschema/migration:" + label.
// Different label strings → different lock IDs → independent serialisation
// scopes (e.g. pgmapbroker migrations never block pgstreambroker migrations).
func AcquireMigrationLock(ctx context.Context, p *pgxpool.Pool, label string) (release func(), err error) {
	lockID := migrationLockID(label)
	conn, err := p.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: acquire migration lock connection: %w", label, err)
	}
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockID); err != nil {
		conn.Release()
		return nil, fmt.Errorf("%s: pg_advisory_lock(%d): %w", label, lockID, err)
	}
	return func() {
		// context.Background() so cleanup runs even when the caller's ctx is
		// already canceled. Best-effort: a dead connection still gets reaped
		// server-side, releasing the lock at session end.
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, lockID)
		conn.Release()
	}, nil
}

// migrationLockID returns a deterministic int64 lock ID for `label`. Different
// labels get different IDs so unrelated consumers (e.g. pgmapbroker vs
// pgstreambroker) never share a serialisation scope.
func migrationLockID(label string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("pgschema/migration:" + label))
	// Postgres advisory locks take an int64. Cast preserves all 64 bits.
	return int64(h.Sum64())
}

// SetSchemaVersion updates schema_version=v in every table listed. Returns
// an error if any UPDATE fails — used at the end of EnsureSchema to bring
// the fresh-install baseline (which the DDL initialised at value 1) up to
// the current schemaVersion. Failing softly here would leave a fresh install
// stuck at version 1 and re-run the entire migration chain on next start.
func SetSchemaVersion(ctx context.Context, q querier, label string, v int, versionTables []string) error {
	for _, t := range versionTables {
		if _, err := q.Exec(ctx, fmt.Sprintf(
			`UPDATE %s SET schema_version = $1 WHERE id = 1`, t), v); err != nil {
			return fmt.Errorf("%s: set schema_version=%d (%s): %w", label, v, t, err)
		}
	}
	return nil
}
