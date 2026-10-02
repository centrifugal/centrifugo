// Package pgshard computes the shard of a channel exactly as the PostgreSQL
// brokers' SQL functions do, so Go code can pick the replica which serves a
// shard.
package pgshard

import (
	"context"
	"fmt"
	"math/bits"

	"github.com/jackc/pgx/v5"
)

// Of returns the shard of channel among numShards, the same as the SQL
// expression abs(hashtext(channel)::bigint) % num_shards.
func Of(channel string, numShards int) int {
	h := int64(int32(hashBytes([]byte(channel))))
	if h < 0 {
		h = -h
	}
	return int(h % int64(numShards))
}

// Querier runs a query returning one row, as a pgx pool does.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// checkKeys are hashed by Check: an ASCII one, and one with a character
// whose bytes change when the connection converts encodings.
var checkKeys = []string{"chat:room", "chat:é"}

// Check reports an error when the database's hashtext() of channels sent over
// db differs from the one Of computes: replica reads would then not follow
// the shards SQL assigns. It happens with a big-endian server, or when the
// connection converts non-ASCII channels to a server encoding other than
// UTF8, as with client_encoding=UTF8 set for a LATIN1 database. By default
// the client encoding is the database one, and channels are hashed as sent.
func Check(ctx context.Context, db Querier) error {
	for _, key := range checkKeys {
		var dbHash int32
		if err := db.QueryRow(ctx, "SELECT hashtext($1)", key).Scan(&dbHash); err != nil {
			return fmt.Errorf("hashtext(%q): %w", key, err)
		}
		if goHash := int32(hashBytes([]byte(key))); dbHash != goHash {
			return fmt.Errorf("hashtext(%q) is %d in the database but %d in Centrifugo", key, dbHash, goHash)
		}
	}
	return nil
}

// hashBytes is PostgreSQL's hash_bytes (src/common/hashfn.c), Bob Jenkins'
// lookup3 hash, on which hashtext() is built for deterministic collations.
// This is the little-endian variant: keys are read as little-endian words
// regardless of the platform, as PostgreSQL does on the platforms it is run
// on with Centrifugo.
func hashBytes(k []byte) uint32 {
	length := uint32(len(k))
	a := 0x9e3779b9 + length + 3923095
	b, c := a, a

	for len(k) >= 12 {
		a += le32(k[0:])
		b += le32(k[4:])
		c += le32(k[8:])
		a, b, c = mix(a, b, c)
		k = k[12:]
	}

	// The lowest byte of c is reserved for the length, so the bytes left
	// for c are added from the second one.
	switch len(k) {
	case 11:
		c += uint32(k[10]) << 24
		fallthrough
	case 10:
		c += uint32(k[9]) << 16
		fallthrough
	case 9:
		c += uint32(k[8]) << 8
		fallthrough
	case 8:
		b += le32(k[4:])
		a += le32(k[0:])
	case 7:
		b += uint32(k[6]) << 16
		fallthrough
	case 6:
		b += uint32(k[5]) << 8
		fallthrough
	case 5:
		b += uint32(k[4])
		fallthrough
	case 4:
		a += le32(k[0:])
	case 3:
		a += uint32(k[2]) << 16
		fallthrough
	case 2:
		a += uint32(k[1]) << 8
		fallthrough
	case 1:
		a += uint32(k[0])
	}

	_, _, c = final(a, b, c)
	return c
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func rot(x uint32, k int) uint32 { return bits.RotateLeft32(x, k) }

func mix(a, b, c uint32) (uint32, uint32, uint32) {
	a -= c
	a ^= rot(c, 4)
	c += b
	b -= a
	b ^= rot(a, 6)
	a += c
	c -= b
	c ^= rot(b, 8)
	b += a
	a -= c
	a ^= rot(c, 16)
	c += b
	b -= a
	b ^= rot(a, 19)
	a += c
	c -= b
	c ^= rot(b, 4)
	b += a
	return a, b, c
}

func final(a, b, c uint32) (uint32, uint32, uint32) {
	c ^= b
	c -= rot(b, 14)
	a ^= c
	a -= rot(c, 11)
	b ^= a
	b -= rot(a, 25)
	c ^= b
	c -= rot(b, 16)
	a ^= c
	a -= rot(c, 4)
	b ^= a
	b -= rot(a, 14)
	c ^= b
	c -= rot(b, 24)
	return a, b, c
}
