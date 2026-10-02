//go:build integration

package pgshard

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestCheckAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("CENTRIFUGE_POSTGRES_URL")
	if dsn == "" {
		dsn = "postgres://test:test@localhost:5432/test?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	defer pool.Close()
	require.NoError(t, Check(context.Background(), pool))

	// A connection converting channels to another encoding hashes non-ASCII
	// ones differently from Centrifugo, whatever the database encoding.
	conf, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	conf.ConnConfig.RuntimeParams["client_encoding"] = "LATIN1"
	converting, err := pgxpool.NewWithConfig(context.Background(), conf)
	require.NoError(t, err)
	defer converting.Close()
	var serverEncoding string
	require.NoError(t, converting.QueryRow(context.Background(), "SHOW server_encoding").Scan(&serverEncoding))
	if serverEncoding == "LATIN1" {
		t.Skip("the database is LATIN1 itself: nothing to convert")
	}
	require.ErrorIs(t, Check(context.Background(), converting), ErrMismatch)
}
