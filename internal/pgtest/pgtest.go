// Package pgtest contains helpers for tests running against a real PostgreSQL
// (see docker-compose.yml). It must only be imported from tests.
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// DSN returns the connection string of the test PostgreSQL: CENTRIFUGE_POSTGRES_URL
// if set, otherwise the docker-compose default.
func DSN() string {
	dsn := os.Getenv("CENTRIFUGE_POSTGRES_URL")
	if dsn == "" {
		dsn = "postgres://test:test@localhost:5432/test?sslmode=disable" //nolint:gosec // Credentials of docker-compose.yml.
	}
	return dsn
}

var schemaDSNs sync.Map // testing.TB -> string

// SchemaDSN creates a PostgreSQL schema private to tb and returns a DSN whose
// search_path points to it, so every table, function and partition created
// through that DSN is invisible to other tests. This lets tests creating
// objects with fixed names (default table prefixes) run in parallel. The
// schema is dropped with everything in it when tb finishes. Repeated calls
// with the same tb return the same DSN, so several brokers created by one
// test share their tables like nodes of one cluster do.
//
// Objects outside the schema stay shared: advisory locks, LISTEN/NOTIFY
// channels, and catalog queries filtering by relation name only.
func SchemaDSN(tb testing.TB) string {
	tb.Helper()
	if dsn, ok := schemaDSNs.Load(tb); ok {
		return dsn.(string)
	}
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		tb.Fatal(err)
	}
	schema := "cftest_" + hex.EncodeToString(b[:])
	base := DSN()
	exec(tb, base, "CREATE SCHEMA "+schema)
	tb.Cleanup(func() {
		schemaDSNs.Delete(tb)
		exec(tb, base, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	})
	dsn := withSearchPath(tb, base, schema)
	schemaDSNs.Store(tb, dsn)
	return dsn
}

func exec(tb testing.TB, dsn string, sql string) {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		tb.Fatalf("pgtest: connect: %v", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(ctx, sql); err != nil {
		tb.Fatalf("pgtest: %s: %v", sql, err)
	}
}

func withSearchPath(tb testing.TB, dsn string, schema string) string {
	tb.Helper()
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		// Keyword/value connection string.
		return dsn + " search_path=" + schema
	}
	u, err := url.Parse(dsn)
	if err != nil {
		tb.Fatalf("pgtest: parse DSN: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}
