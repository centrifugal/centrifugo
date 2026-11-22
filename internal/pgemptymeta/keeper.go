// Package pgemptymeta keeps the meta rows of channels without publications in
// the PostgreSQL brokers.
//
// Such a row gives the channel an epoch: a position with an empty epoch would
// match any later stream. It may have a short TTL (HistoryMetaTTLUnpublished),
// and Keeper extends it while the channel has subscribers on the node. In
// Redis position checks do that, but centrifuge skips them for brokers with
// ReliableDelivery. If the row expired under a subscriber, the first
// publication would come with another epoch and be an insufficient state.
package pgemptymeta

import (
	"context"
	"fmt"
	"hash/maphash"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// BatchSize is how many channels one statement of Keep covers.
	BatchSize = 1000
	// maxWait bounds trust in a remembered expiry, for a database restored or
	// changed by hand.
	maxWait   = time.Hour
	numShards = 64
)

// RefreshInSQL is in how many seconds meta row m has half of its TTL left. The
// TTL depends on the channel, so it is taken from the row: expires_at minus
// updated_at. NULL for a row with publications, whose expires_at may be
// infinite, and for one which doesn't expire.
const RefreshInSQL = `CASE WHEN m.top_offset = 0 THEN
	EXTRACT(EPOCH FROM (m.expires_at - NOW()) - (m.expires_at - m.updated_at) / 2)::float8 END`

// TTL returns how long the meta of a channel without publications is kept:
// metaTTL, or unpublished if that is set and shorter. It is at least 3 position
// check delays, so that Keeper can keep it. Zero means it doesn't expire.
func TTL(metaTTL, unpublished, checkDelay time.Duration) time.Duration {
	ttl := metaTTL
	if unpublished > 0 && (ttl == 0 || unpublished < ttl) {
		ttl = unpublished
	}
	if ttl == 0 {
		return 0
	}
	return max(ttl, 3*checkDelay)
}

// Keeper holds subscribed channels without publications, with the time each is
// to be looked at, and queries only the ones which are due. This is safe as the
// expiry of such a row never moves back: writes extend it and commit
// synchronously, and for the same reason reads don't shorten it.
type Keeper struct {
	// HasSubscribers reports whether a channel has subscribers on this node.
	HasSubscribers func(ch string) bool

	pool   *pgxpool.Pool
	table  string
	seed   maphash.Seed
	shards [numShards]shard
}

type shard struct {
	mu  sync.Mutex
	due map[string]int64 // Unix nanoseconds
}

// New creates a Keeper for the meta table.
func New(pool *pgxpool.Pool, table string, hasSubscribers func(ch string) bool) *Keeper {
	k := &Keeper{HasSubscribers: hasSubscribers, pool: pool, table: table, seed: maphash.MakeSeed()}
	for i := range k.shards {
		k.shards[i].due = map[string]int64{}
	}
	return k
}

func (k *Keeper) shard(ch string) *shard {
	return &k.shards[maphash.String(k.seed, ch)%numShards]
}

// Watch is called for each read of an empty channel. Centrifuge adds a
// subscriber before it reads the position, so a subscriber's read is seen.
func (k *Keeper) Watch(ch string, refreshIn float64, now time.Time) {
	if k.HasSubscribers(ch) {
		k.Schedule(ch, refreshIn, now)
	}
}

// Schedule has the channel looked at in refreshIn seconds.
func (k *Keeper) Schedule(ch string, refreshIn float64, now time.Time) {
	wait := min(max(refreshIn, 0), maxWait.Seconds())
	s := k.shard(ch)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.due[ch] = now.UnixNano() + int64(wait*float64(time.Second))
}

// Forget drops channels.
func (k *Keeper) Forget(channels ...string) {
	for _, ch := range channels {
		s := k.shard(ch)
		s.mu.Lock()
		delete(s.due, ch)
		s.mu.Unlock()
	}
}

// DueChannels returns the channels to look at.
func (k *Keeper) DueChannels(now time.Time) []string {
	var res []string
	ts := now.UnixNano()
	for i := range k.shards {
		s := &k.shards[i]
		s.mu.Lock()
		for ch, due := range s.due {
			if due <= ts {
				res = append(res, ch)
			}
		}
		s.mu.Unlock()
	}
	return res
}

// Run calls Keep twice per position check delay until ctx is done: with the
// shortest TTL, 3 delays, one failed run still doesn't lose a row.
func (k *Keeper) Run(ctx context.Context, checkDelay time.Duration, onError func(error)) {
	ticker := time.NewTicker(checkDelay / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := k.Keep(ctx, time.Now()); err != nil && ctx.Err() == nil {
				onError(err)
			}
		}
	}
}

// Keep extends the due rows with less than half of their TTL left.
//
// SKIP LOCKED, as several nodes keep the same channels: a row locked by one of
// them, or by a publish, stays due and is looked at by the next run.
func (k *Keeper) Keep(ctx context.Context, now time.Time) error {
	var channels []string
	for _, ch := range k.DueChannels(now) {
		if k.HasSubscribers(ch) {
			channels = append(channels, ch)
		} else {
			k.Forget(ch)
		}
	}
	if len(channels) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
		WITH due AS MATERIALIZED (
			SELECT channel FROM %[1]s
			 WHERE channel = ANY($1)
			   AND expires_at < NOW() + CASE WHEN top_offset = 0 THEN (expires_at - updated_at) / 2 END
			   FOR UPDATE SKIP LOCKED
		), kept AS (
			UPDATE %[1]s AS m
			   SET expires_at = NOW() + (m.expires_at - m.updated_at), updated_at = NOW()
			  FROM due
			 WHERE m.channel = due.channel
			RETURNING m.channel, %[2]s AS refresh_in
		)
		SELECT m.channel, COALESCE(kept.refresh_in, %[2]s)
		  FROM %[1]s AS m LEFT JOIN kept ON kept.channel = m.channel
		 WHERE m.channel = ANY($1)
	`, k.table, RefreshInSQL)

	for batch := range slices.Chunk(channels, BatchSize) {
		if err := k.keepBatch(ctx, query, batch, now); err != nil {
			return err
		}
	}
	return nil
}

func (k *Keeper) keepBatch(ctx context.Context, query string, batch []string, now time.Time) error {
	rows, err := k.pool.Query(ctx, query, batch)
	if err != nil {
		return err
	}
	defer rows.Close()
	empty := make(map[string]float64, len(batch))
	for rows.Next() {
		var ch string
		var refreshIn *float64
		if err := rows.Scan(&ch, &refreshIn); err != nil {
			return err
		}
		if refreshIn != nil {
			empty[ch] = *refreshIn
		}
	}
	// Nothing is rescheduled before the commit is known to have succeeded.
	if err := rows.Err(); err != nil {
		return err
	}
	for _, ch := range batch {
		if refreshIn, ok := empty[ch]; ok {
			k.Schedule(ch, refreshIn, now)
		} else {
			k.Forget(ch) // has publications, doesn't expire, or has no meta any more
		}
	}
	return nil
}
