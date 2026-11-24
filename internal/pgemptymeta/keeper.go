// Package pgemptymeta keeps the meta rows of channels without publications in
// the PostgreSQL brokers.
//
// Such a row gives the channel an epoch: a position with an empty epoch would
// match any later stream. It may have a short TTL (HistoryMetaTTLInitial),
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
// metaTTL without the initial TTL, otherwise the initial TTL if it is shorter,
// at least 3 position check delays so that Keeper can keep it. Zero means it
// doesn't expire.
func TTL(metaTTL, initial, checkDelay time.Duration) time.Duration {
	if initial <= 0 {
		return metaTTL
	}
	ttl := metaTTL
	if ttl == 0 || initial < ttl {
		ttl = initial
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
//
// Subscribes and reads schedule channels while a run works, so a run applies
// what it saw only to the schedule it took: what it saw may be older.
type Keeper struct {
	// HasSubscribers reports whether a channel has subscribers on this node.
	HasSubscribers func(ch string) bool

	pool   *pgxpool.Pool
	table  string
	seed   maphash.Seed
	shards [numShards]shard
}

type shard struct {
	mu      sync.Mutex
	lastGen uint64
	entries map[string]entry
}

type entry struct {
	at  int64 // Unix nanoseconds
	gen uint64
}

type dueChannel struct {
	name string
	gen  uint64
}

// New creates a Keeper for the meta table. Brokers make it with Lazy. A nil
// Keeper does nothing.
func New(pool *pgxpool.Pool, table string, hasSubscribers func(ch string) bool) *Keeper {
	k := &Keeper{HasSubscribers: hasSubscribers, pool: pool, table: table, seed: maphash.MakeSeed()}
	for i := range k.shards {
		k.shards[i].entries = map[string]entry{}
	}
	return k
}

func (k *Keeper) shard(ch string) *shard {
	return &k.shards[maphash.String(k.seed, ch)%numShards]
}

// Watch is called for each read of an empty channel. Centrifuge adds a
// subscriber before it reads the position, so a subscriber's read is seen.
func (k *Keeper) Watch(ch string, refreshIn float64, now time.Time) {
	if k == nil {
		return
	}
	if k.HasSubscribers(ch) {
		k.Schedule(ch, refreshIn, now)
	}
}

// Schedule has the channel looked at in refreshIn seconds.
func (k *Keeper) Schedule(ch string, refreshIn float64, now time.Time) {
	if k == nil {
		return
	}
	s := k.shard(ch)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.set(ch, refreshIn, now)
}

func (s *shard) set(ch string, refreshIn float64, now time.Time) {
	wait := min(max(refreshIn, 0), maxWait.Seconds())
	s.lastGen++
	s.entries[ch] = entry{at: now.UnixNano() + int64(wait*float64(time.Second)), gen: s.lastGen}
}

// reschedule applies what a run saw, nil to drop the channel, unless it was
// scheduled or forgotten since the run took it.
func (k *Keeper) reschedule(c dueChannel, refreshIn *float64, now time.Time) {
	s := k.shard(c.name)
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[c.name]; !ok || e.gen != c.gen {
		return
	}
	if refreshIn == nil {
		delete(s.entries, c.name)
		return
	}
	s.set(c.name, *refreshIn, now)
}

// Forget drops channels.
func (k *Keeper) Forget(channels ...string) {
	if k == nil {
		return
	}
	for _, ch := range channels {
		s := k.shard(ch)
		s.mu.Lock()
		delete(s.entries, ch)
		s.mu.Unlock()
	}
}

// IsDue reports whether the channel is to be looked at by now.
func (k *Keeper) IsDue(ch string, now time.Time) bool {
	s := k.shard(ch)
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[ch]
	return ok && e.at <= now.UnixNano()
}

func (k *Keeper) due(now time.Time) []dueChannel {
	var res []dueChannel
	ts := now.UnixNano()
	for i := range k.shards {
		s := &k.shards[i]
		s.mu.Lock()
		for ch, e := range s.entries {
			if e.at <= ts {
				res = append(res, dueChannel{name: ch, gen: e.gen})
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
	var channels []dueChannel
	for _, c := range k.due(now) {
		if k.HasSubscribers(c.name) {
			channels = append(channels, c)
		} else {
			k.reschedule(c, nil, now)
		}
	}
	if len(channels) == 0 {
		return nil
	}

	// Rows are looked up one channel at a time, by the primary key. With
	// channel = ANY($1) the planner may scan the table and compare each row
	// with the whole batch, which made a batch's cost grow with the table.
	// The locking clause keeps the first lookup from being turned into a join,
	// OFFSET 0 the second.
	query := fmt.Sprintf(`
		WITH names AS MATERIALIZED (
			SELECT unnest($1::text[]) AS channel
		), due AS MATERIALIZED (
			SELECT d.channel FROM names CROSS JOIN LATERAL (
				SELECT channel FROM %[1]s
				 WHERE channel = names.channel
				   AND expires_at < NOW() + CASE WHEN top_offset = 0 THEN (expires_at - updated_at) / 2 END
				   FOR UPDATE SKIP LOCKED
			) AS d
		), kept AS (
			UPDATE %[1]s AS m
			   SET expires_at = NOW() + (m.expires_at - m.updated_at), updated_at = NOW()
			  FROM due
			 WHERE m.channel = due.channel
			RETURNING m.channel, %[2]s AS refresh_in
		)
		SELECT names.channel, COALESCE(kept.refresh_in, cur.refresh_in)
		  FROM names
		 CROSS JOIN LATERAL (
			SELECT %[2]s AS refresh_in FROM %[1]s AS m WHERE m.channel = names.channel OFFSET 0
		 ) AS cur
		  LEFT JOIN kept ON kept.channel = names.channel
	`, k.table, RefreshInSQL)

	for batch := range slices.Chunk(channels, BatchSize) {
		if err := k.keepBatch(ctx, query, batch, now); err != nil {
			return err
		}
	}
	return nil
}

func (k *Keeper) keepBatch(ctx context.Context, query string, batch []dueChannel, now time.Time) error {
	names := make([]string, len(batch))
	for i, c := range batch {
		names[i] = c.name
	}
	rows, err := k.pool.Query(ctx, query, names)
	if err != nil {
		return err
	}
	defer rows.Close()
	seen := make(map[string]*float64, len(batch))
	for rows.Next() {
		var ch string
		var refreshIn *float64
		if err := rows.Scan(&ch, &refreshIn); err != nil {
			return err
		}
		seen[ch] = refreshIn
	}
	// Nothing is rescheduled before the commit is known to have succeeded.
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range batch {
		// nil: it has publications, doesn't expire, or has no meta any more.
		k.reschedule(c, seen[c.name], now)
	}
	return nil
}
