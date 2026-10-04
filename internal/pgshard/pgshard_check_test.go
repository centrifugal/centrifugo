package pgshard

import (
	"context"
	"errors"
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
	err := Check(context.Background(), latin1)
	require.ErrorIs(t, err, ErrMismatch)
	require.ErrorContains(t, err, `hashtext("chat:é") is -1932169419 in the database`)

	// Every key differs on a big-endian server, ASCII ones included.
	bigEndian := hashQuerier(func(key string) int32 { return int32(hashBytes([]byte(key))) + 1 })
	require.ErrorIs(t, Check(context.Background(), bigEndian), ErrMismatch)
}

type failingQuerier struct{}

type failingRow struct{}

func (failingRow) Scan(...any) error { return errors.New("connection refused") }

func (failingQuerier) QueryRow(context.Context, string, ...any) pgx.Row { return failingRow{} }

// A database the check can't be run with is not reported as one which
// shards differently.
func TestLogCheck(t *testing.T) {
	var msgs []string
	logError := func(msg string, _ error) { msgs = append(msgs, msg) }

	LogCheck(context.Background(), hashQuerier(func(key string) int32 { return int32(hashBytes([]byte(key))) }), logError)
	require.Empty(t, msgs)

	LogCheck(context.Background(), failingQuerier{}, logError)
	require.Equal(t, []string{"could not check how the database shards channels"}, msgs)

	msgs = nil
	LogCheck(context.Background(), hashQuerier(func(string) int32 { return 1 }), logError)
	require.Equal(t, []string{"replica reads may not follow live delivery"}, msgs)
}
