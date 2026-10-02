// Package pgshard computes the shard of a channel exactly as the PostgreSQL
// brokers' SQL functions do, so Go code can pick the replica which serves a
// shard.
package pgshard

import "math/bits"

// Of returns the shard of channel among numShards, the same as the SQL
// expression abs(hashtext(channel)::bigint) % num_shards.
func Of(channel string, numShards int) int {
	h := int64(int32(hashBytes([]byte(channel))))
	if h < 0 {
		h = -h
	}
	return int(h % int64(numShards))
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
