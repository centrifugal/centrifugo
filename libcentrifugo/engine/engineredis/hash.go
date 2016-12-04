package engineredis

import (
	"hash/fnv"
)

// Adapted from from https://github.com/dgryski/go-jump package by Damian Gryski.
// Hash consistently chooses a hash bucket number in the range [0, numBuckets) for the given key.
// numBuckets must be >= 1.
func Hash(ch string, numBuckets int) int32 {

	hash := fnv.New64a()
	hash.Write([]byte(ch))
	key := hash.Sum64()

	var b int64 = -1
	var j int64

	for j < int64(numBuckets) {
		b = j
		key = key*2862933555777941757 + 1
		j = int64(float64(b+1) * (float64(int64(1)<<31) / float64((key>>33)+1)))
	}

	return int32(b)
}
