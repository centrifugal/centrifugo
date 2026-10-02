package pgshard

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type hashRow struct{ hash int32 }

func (r hashRow) Scan(dest ...any) error {
	*dest[0].(*int32) = r.hash
	return nil
}

// hashQuerier answers hashtext() with a fixed function, as a database would.
type hashQuerier func(key string) int32

func (q hashQuerier) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	return hashRow{hash: q(args[0].(string))}
}

func TestCheck(t *testing.T) {
	same := hashQuerier(func(key string) int32 { return int32(hashBytes([]byte(key))) })
	require.NoError(t, Check(context.Background(), same))

	// hashtext('chat:é') of a LATIN1 database, when the connection converts
	// it from UTF8.
	latin1 := hashQuerier(func(key string) int32 {
		if key == "chat:é" {
			return -1932169419
		}
		return int32(hashBytes([]byte(key)))
	})
	require.ErrorContains(t, Check(context.Background(), latin1), `hashtext("chat:é") is -1932169419 in the database`)
}
