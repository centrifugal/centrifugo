package pgschema

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// ----- ValidateMigrationMap -----

func TestValidateMigrationMap_Contiguous_OK(t *testing.T) {
	require.NotPanics(t, func() {
		ValidateMigrationMap("test", 3, map[int]string{
			2: "-- migration v2",
			3: "-- migration v3",
		})
	})
}

func TestValidateMigrationMap_EmptyAtV1_OK(t *testing.T) {
	require.NotPanics(t, func() {
		ValidateMigrationMap("test", 1, map[int]string{})
	})
}

func TestValidateMigrationMap_MissingEntry_Panics(t *testing.T) {
	require.PanicsWithValue(t,
		"test: missing schemaMigrations[2] (schemaVersion=3) — every version 2..schemaVersion must have a migration registered",
		func() {
			ValidateMigrationMap("test", 3, map[int]string{3: "-- v3 only"})
		})
}

func TestValidateMigrationMap_OutOfRange_Panics(t *testing.T) {
	// Version 5 is out of range when target is 3.
	require.Panics(t, func() {
		ValidateMigrationMap("test", 3, map[int]string{
			2: "-- v2",
			3: "-- v3",
			5: "-- stray v5",
		})
	})
}

func TestValidateMigrationMap_Version1_Panics(t *testing.T) {
	// Version 1 is the baseline (DDL), not a migration target.
	require.Panics(t, func() {
		ValidateMigrationMap("test", 1, map[int]string{1: "-- bad v1"})
	})
}

// ----- CheckDowngrade -----

func TestCheckDowngrade_EqualOrLower_OK(t *testing.T) {
	require.NoError(t, CheckDowngrade("x", 1, 3))
	require.NoError(t, CheckDowngrade("x", 3, 3))
	require.NoError(t, CheckDowngrade("x", 0, 3))
}

func TestCheckDowngrade_DBNewer_Rejected(t *testing.T) {
	err := CheckDowngrade("pgmapbroker", 5, 3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "schema_version is 5")
	require.Contains(t, err.Error(), "supports only up to 3")
	require.Contains(t, err.Error(), "downgrade not supported")
}

// ----- Integration helpers -----
//
// The remaining tests need a real Postgres because the queries hit table-
// existence/permission paths that no fake-row implementation can simulate
// faithfully. Skip when not configured.

func getPostgresConnString(tb testing.TB) string {
	tb.Helper()
	// Match the env var the broker test suites use so a single local Postgres
	// runs the whole suite.
	dsn := os.Getenv("CENTRIFUGE_POSTGRES_URL")
	if dsn == "" {
		dsn = "postgres://test:test@localhost:5432/test?sslmode=disable"
	}
	// Probe before each test so a missing DB skips, not fails.
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		tb.Skipf("invalid PG_TEST_DSN: %v", err)
	}
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		tb.Skipf("postgres not reachable: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		tb.Skipf("postgres ping failed: %v", err)
	}
	return dsn
}

// newTestPool gives each test a private prefix to avoid cross-test races.
func newTestPool(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := getPostgresConnString(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	// Use a stable but per-test prefix derived from the test name. Postgres
	// identifiers max 63 bytes — truncate just in case.
	prefix := "pgschema_test_" + sanitize(t.Name()) + "_"
	if len(prefix) > 50 {
		prefix = prefix[:50] + "_"
	}
	cleanup(t, pool, prefix)
	t.Cleanup(func() { cleanup(t, pool, prefix) })
	return pool, prefix
}

func sanitize(s string) string {
	r := strings.NewReplacer("/", "_", " ", "_", "-", "_")
	return strings.ToLower(r.Replace(s))
}

func cleanup(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %sschema_version CASCADE", prefix))
	_, _ = pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %sprobe CASCADE", prefix))
}

// createVersionTable creates a `<prefix>schema_version` table seeded at value `v`.
func createVersionTable(t *testing.T, pool *pgxpool.Pool, prefix string, v int) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %sschema_version (
			id              INTEGER PRIMARY KEY,
			schema_version  INTEGER NOT NULL
		)`, prefix))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %sschema_version (id, schema_version) VALUES (1, $1)`, prefix), v)
	require.NoError(t, err)
}

// ----- ReadSchemaVersion -----

func TestReadSchemaVersion_RowPresent(t *testing.T) {
	pool, prefix := newTestPool(t)
	createVersionTable(t, pool, prefix, 3)
	v, isFresh, err := ReadSchemaVersion(context.Background(), pool, prefix+"schema_version")
	require.NoError(t, err)
	require.False(t, isFresh)
	require.Equal(t, 3, v)
}

func TestReadSchemaVersion_TableMissing_TreatedAsFresh(t *testing.T) {
	pool, prefix := newTestPool(t)
	// Don't create the table.
	v, isFresh, err := ReadSchemaVersion(context.Background(), pool, prefix+"schema_version")
	require.NoError(t, err)
	require.True(t, isFresh)
	require.Equal(t, 0, v)
}

func TestReadSchemaVersion_RowMissing_TreatedAsFresh(t *testing.T) {
	pool, prefix := newTestPool(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %sschema_version (
			id              INTEGER PRIMARY KEY,
			schema_version  INTEGER NOT NULL
		)`, prefix))
	require.NoError(t, err)
	// No INSERT — row missing.
	v, isFresh, err := ReadSchemaVersion(ctx, pool, prefix+"schema_version")
	require.NoError(t, err)
	require.True(t, isFresh)
	require.Equal(t, 0, v)
}

// TestReadSchemaVersion_ColumnMissing_PropagatesError verifies the critical
// safety property: a column-missing error (any error other than 42P01 or
// ErrNoRows) must propagate, NOT be silently treated as "fresh install" and
// force schema_version forward. Stand-in for the broader class of
// transient/permission/timeout failures.
func TestReadSchemaVersion_ColumnMissing_PropagatesError(t *testing.T) {
	pool, prefix := newTestPool(t)
	ctx := context.Background()
	// Create the table but with the WRONG column name. Reading `schema_version`
	// then errors with 42703 (undefined_column) — neither ErrNoRows nor 42P01.
	_, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %sschema_version (
			id              INTEGER PRIMARY KEY,
			wrong_column    INTEGER NOT NULL
		)`, prefix))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %sschema_version (id, wrong_column) VALUES (1, 3)`, prefix))
	require.NoError(t, err)

	v, isFresh, err := ReadSchemaVersion(ctx, pool, prefix+"schema_version")
	require.Error(t, err, "transient/non-fresh errors must propagate")
	require.False(t, isFresh, "isFresh must be false when an error is returned")
	require.Equal(t, 0, v)
	// Verify it surfaces as a pg error (column missing).
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr))
	require.Equal(t, "42703", pgErr.Code)
}

// ----- ApplyMigrationInTx -----

func TestApplyMigrationInTx_SuccessUpdatesVersion(t *testing.T) {
	pool, prefix := newTestPool(t)
	createVersionTable(t, pool, prefix, 1)
	ctx := context.Background()

	// Migration adds a column to a probe table.
	_, err := pool.Exec(ctx, fmt.Sprintf(`CREATE TABLE %sprobe (id INTEGER)`, prefix))
	require.NoError(t, err)

	migration := fmt.Sprintf(`ALTER TABLE %sprobe ADD COLUMN extra TEXT`, prefix)

	err = ApplyMigrationInTx(ctx, pool, "test", 2, []MigrationVariant{
		{SQL: migration, VersionTable: prefix + "schema_version"},
	})
	require.NoError(t, err)

	// Verify migration applied.
	var colExists bool
	err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = 'extra')`,
		prefix+"probe").Scan(&colExists)
	require.NoError(t, err)
	require.True(t, colExists)

	// Verify schema_version bumped.
	var v int
	err = pool.QueryRow(ctx, fmt.Sprintf(`SELECT schema_version FROM %sschema_version WHERE id = 1`, prefix)).Scan(&v)
	require.NoError(t, err)
	require.Equal(t, 2, v)
}

// TestApplyMigrationInTx_FailureRollsBack is the key safety test: a failed
// migration must NOT leave schema_version bumped, and must NOT leave partial
// schema changes committed. Atomicity is the whole point of running migration
// + version bump in one transaction.
func TestApplyMigrationInTx_FailureRollsBack(t *testing.T) {
	pool, prefix := newTestPool(t)
	createVersionTable(t, pool, prefix, 1)
	ctx := context.Background()

	_, err := pool.Exec(ctx, fmt.Sprintf(`CREATE TABLE %sprobe (id INTEGER)`, prefix))
	require.NoError(t, err)

	// Migration adds a column AND then runs invalid SQL — the whole tx must roll back.
	migration := fmt.Sprintf(`
		ALTER TABLE %sprobe ADD COLUMN extra TEXT;
		ALTER TABLE %sprobe ADD COLUMN extra TEXT;  -- duplicate, fails
	`, prefix, prefix)

	err = ApplyMigrationInTx(ctx, pool, "test", 2, []MigrationVariant{
		{SQL: migration, VersionTable: prefix + "schema_version"},
	})
	require.Error(t, err)

	// Verify the first ADD COLUMN was rolled back — `extra` must NOT exist.
	var colExists bool
	err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = 'extra')`,
		prefix+"probe").Scan(&colExists)
	require.NoError(t, err)
	require.False(t, colExists, "ADD COLUMN must have rolled back")

	// Verify schema_version unchanged.
	var v int
	err = pool.QueryRow(ctx, fmt.Sprintf(`SELECT schema_version FROM %sschema_version WHERE id = 1`, prefix)).Scan(&v)
	require.NoError(t, err)
	require.Equal(t, 1, v, "schema_version must not have advanced after rollback")
}

func TestApplyMigrationInTx_MissingRowFailsLoudly(t *testing.T) {
	pool, prefix := newTestPool(t)
	ctx := context.Background()
	// Create version table but DON'T insert a row.
	_, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %sschema_version (
			id              INTEGER PRIMARY KEY,
			schema_version  INTEGER NOT NULL
		)`, prefix))
	require.NoError(t, err)

	err = ApplyMigrationInTx(ctx, pool, "test", 2, []MigrationVariant{
		{SQL: `SELECT 1`, VersionTable: prefix + "schema_version"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no row id=1")
}

func TestApplyMigrationInTx_MultipleVariants(t *testing.T) {
	pool, prefix := newTestPool(t)
	createVersionTable(t, pool, prefix, 1)
	prefix2 := prefix + "alt_"
	t.Cleanup(func() { cleanup(t, pool, prefix2) })
	createVersionTable(t, pool, prefix2, 1)
	ctx := context.Background()

	err := ApplyMigrationInTx(ctx, pool, "test", 2, []MigrationVariant{
		{SQL: `SELECT 1`, VersionTable: prefix + "schema_version"},
		{SQL: `SELECT 1`, VersionTable: prefix2 + "schema_version"},
	})
	require.NoError(t, err)

	for _, p := range []string{prefix, prefix2} {
		var v int
		err = pool.QueryRow(ctx, fmt.Sprintf(`SELECT schema_version FROM %sschema_version WHERE id = 1`, p)).Scan(&v)
		require.NoError(t, err)
		require.Equal(t, 2, v, "prefix %s should be at v2", p)
	}
}

// ----- SetSchemaVersion -----

func TestSetSchemaVersion_UpdatesAllTables(t *testing.T) {
	pool, prefix := newTestPool(t)
	createVersionTable(t, pool, prefix, 1)
	ctx := context.Background()

	require.NoError(t, SetSchemaVersion(ctx, pool, "test", 5, []string{prefix + "schema_version"}))

	var v int
	err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT schema_version FROM %sschema_version WHERE id = 1`, prefix)).Scan(&v)
	require.NoError(t, err)
	require.Equal(t, 5, v)
}

func TestSetSchemaVersion_TableMissing_PropagatesError(t *testing.T) {
	pool, prefix := newTestPool(t)
	// Don't create the table.
	err := SetSchemaVersion(context.Background(), pool, "test", 2, []string{prefix + "schema_version"})
	require.Error(t, err, "UPDATE against a missing table must propagate (no longer silent)")
}

// TestApplyMigrationInTx_NoVariants returns an error rather than running with
// nothing — guards against a programming mistake where a broker forgets to
// build the variant slice.
func TestApplyMigrationInTx_NoVariants(t *testing.T) {
	pool, _ := newTestPool(t)
	err := ApplyMigrationInTx(context.Background(), pool, "test", 2, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no variants")
}

// TestApplyMigrationInTx_PerVariantSQL verifies the per-variant SQL is run
// in the right order against the right tables — distinct migrations for each
// variant landed correctly, and the version row of each variant is bumped.
func TestApplyMigrationInTx_PerVariantSQL(t *testing.T) {
	pool, prefix := newTestPool(t)
	createVersionTable(t, pool, prefix, 1)
	prefix2 := prefix + "alt_"
	t.Cleanup(func() { cleanup(t, pool, prefix2) })
	createVersionTable(t, pool, prefix2, 1)
	ctx := context.Background()

	_, err := pool.Exec(ctx, fmt.Sprintf(`CREATE TABLE %sprobe (id INTEGER)`, prefix))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, fmt.Sprintf(`CREATE TABLE %sprobe (id INTEGER)`, prefix2))
	require.NoError(t, err)

	err = ApplyMigrationInTx(ctx, pool, "test", 2, []MigrationVariant{
		{
			SQL:          fmt.Sprintf(`ALTER TABLE %sprobe ADD COLUMN col_a TEXT`, prefix),
			VersionTable: prefix + "schema_version",
		},
		{
			SQL:          fmt.Sprintf(`ALTER TABLE %sprobe ADD COLUMN col_b TEXT`, prefix2),
			VersionTable: prefix2 + "schema_version",
		},
	})
	require.NoError(t, err)

	// Verify each variant got its own column.
	for _, c := range []struct {
		table, column string
	}{
		{prefix + "probe", "col_a"},
		{prefix2 + "probe", "col_b"},
	} {
		var exists bool
		err = pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)`,
			c.table, c.column,
		).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "column %s should exist on %s", c.column, c.table)
	}

	// Both version tables bumped.
	for _, p := range []string{prefix, prefix2} {
		var v int
		err = pool.QueryRow(ctx, fmt.Sprintf(`SELECT schema_version FROM %sschema_version WHERE id = 1`, p)).Scan(&v)
		require.NoError(t, err)
		require.Equal(t, 2, v)
	}
}

// Ensure we don't accidentally tie the package to pgx-specific row types that
// would break the querier abstraction at compile time.
var _ = pgx.ErrNoRows

// ----- migrationLockID -----

func TestMigrationLockID_Deterministic(t *testing.T) {
	require.Equal(t, migrationLockID("pgmapbroker"), migrationLockID("pgmapbroker"))
}

func TestMigrationLockID_DifferentLabelsDifferentIDs(t *testing.T) {
	// Independent broker labels must get independent serialisation scopes.
	require.NotEqual(t, migrationLockID("pgmapbroker"), migrationLockID("pgstreambroker"))
	require.NotEqual(t, migrationLockID("pgmapbroker"), migrationLockID("postgres-controller"))
}

// ----- AcquireMigrationLock -----

// TestAcquireMigrationLock_SerialisesConcurrentCallers proves the core
// rolling-deploy guarantee at the pgschema layer: two concurrent
// AcquireMigrationLock calls block on each other, only one holds the lock at
// a time. Without this serialisation, two nodes upgrading simultaneously
// could race through the migration loop in parallel.
func TestAcquireMigrationLock_SerialisesConcurrentCallers(t *testing.T) {
	pool, _ := newTestPool(t)
	ctx := context.Background()

	// First caller holds the lock.
	release1, err := AcquireMigrationLock(ctx, pool, "concurrent-test")
	require.NoError(t, err)

	// Second caller attempts to acquire — must block until release1 is called.
	acquired := make(chan struct{})
	go func() {
		release2, err := AcquireMigrationLock(ctx, pool, "concurrent-test")
		require.NoError(t, err)
		close(acquired)
		release2()
	}()

	// Give the goroutine a chance to be waiting on pg_advisory_lock.
	select {
	case <-acquired:
		t.Fatal("second caller acquired lock while first still holds it")
	case <-time.After(150 * time.Millisecond):
	}

	// Release the first lock; second goroutine must now make progress.
	release1()
	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("second caller did not acquire lock after release")
	}
}

// TestAcquireMigrationLock_DifferentLabelsDoNotBlock verifies the scope of
// the lock — two callers with DIFFERENT labels do not block each other (e.g.
// pgmapbroker and pgstreambroker migrations can run in parallel).
func TestAcquireMigrationLock_DifferentLabelsDoNotBlock(t *testing.T) {
	pool, _ := newTestPool(t)
	ctx := context.Background()

	release1, err := AcquireMigrationLock(ctx, pool, "label-a")
	require.NoError(t, err)
	defer release1()

	// label-b must acquire immediately — different lock IDs.
	done := make(chan struct{})
	go func() {
		release2, err := AcquireMigrationLock(ctx, pool, "label-b")
		require.NoError(t, err)
		release2()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("label-b should not be blocked by label-a's lock")
	}
}

// TestAcquireMigrationLock_ReleaseUnlocksPoolConnection verifies the release
// function explicitly unlocks BEFORE returning the connection to the pool —
// so a subsequent unrelated caller acquiring the same connection doesn't
// inherit an active advisory lock.
func TestAcquireMigrationLock_ReleaseUnlocksPoolConnection(t *testing.T) {
	dsn := getPostgresConnString(t)
	// Single-conn pool so the next Acquire definitely reuses the same conn.
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	defer pool.Close()
	ctx := context.Background()

	release, err := AcquireMigrationLock(ctx, pool, "release-test")
	require.NoError(t, err)
	release()

	// After release, the same conn is back in the pool. A second
	// AcquireMigrationLock for the same label must not see the lock as held.
	release2, err := AcquireMigrationLock(ctx, pool, "release-test")
	require.NoError(t, err)
	release2()
}
