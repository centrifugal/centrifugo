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
}
