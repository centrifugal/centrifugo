package tntengine

import (
	"hash/fnv"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
)

// index chooses bucket number in range [0, numBuckets).
func index(s string, numBuckets int) int {
	if numBuckets == 1 {
		return 0
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(s))
	return int(hash.Sum64() % uint64(numBuckets))
}

// consistentIndex is an adapted function from https://github.com/dgryski/go-jump
// package by Damian Gryski. It consistently chooses a hash bucket number in the
// range [0, numBuckets) for the given string. numBuckets must be >= 1.
func consistentIndex(s string, numBuckets int) int {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(s))
	key := hash.Sum64()

	var (
		b int64 = -1
		j int64
	)

	for j < int64(numBuckets) {
		b = j
		key = key*2862933555777941757 + 1
		j = int64(float64(b+1) * (float64(int64(1)<<31) / float64((key>>33)+1)))
	}

	return int(b)
}

func consistentShard(ch string, shards []*Shard) *Shard {
	if len(shards) == 1 {
		return shards[0]
	}
	return shards[consistentIndex(ch, len(shards))]
}

func infoToProto(v *centrifuge.ClientInfo) *protocol.ClientInfo {
	if v == nil {
		return nil
	}
	info := &protocol.ClientInfo{
		Client: v.ClientID,
		User:   v.UserID,
	}
	if len(v.ConnInfo) > 0 {
		info.ConnInfo = v.ConnInfo
	}
	if len(v.ChanInfo) > 0 {
		info.ChanInfo = v.ChanInfo
	}
	return info
}

func infoFromProto(v *protocol.ClientInfo) *centrifuge.ClientInfo {
	if v == nil {
		return nil
	}
	info := &centrifuge.ClientInfo{
		ClientID: v.GetClient(),
		UserID:   v.GetUser(),
	}
	if len(v.ConnInfo) > 0 {
		info.ConnInfo = v.ConnInfo
	}
	if len(v.ChanInfo) > 0 {
		info.ChanInfo = v.ChanInfo
	}
	return info
}

var timerPool sync.Pool

// AcquireTimer from pool.
func AcquireTimer(d time.Duration) *time.Timer {
	v := timerPool.Get()
	if v == nil {
		return time.NewTimer(d)
	}

	tm := v.(*time.Timer)
	if tm.Reset(d) {
		panic("Received an active timer from the pool!")
	}
	return tm
}

// ReleaseTimer to pool.
func ReleaseTimer(tm *time.Timer) {
	if !tm.Stop() {
		// Do not reuse timer that has been already stopped.
		// See https://groups.google.com/forum/#!topic/golang-nuts/-8O3AknKpwk
		return
	}
	timerPool.Put(tm)
}
