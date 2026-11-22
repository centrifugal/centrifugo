package pgstreambroker

import (
	"context"
	"fmt"
	"hash/maphash"
	"slices"
	"sync"
	"time"
)

const (
	emptyMetaBatchSize = 1000
	// emptyMetaMaxWait bounds trust in a remembered expiry, for a database
	// restored or changed by hand.
	emptyMetaMaxWait = time.Hour
	emptyMetaShards  = 64
)

// refreshInSQL is in how many seconds the meta m of a channel without
// publications has half of its TTL left. The TTL depends on the namespace, so
// it is taken from the row: expires_at minus updated_at. Only evaluated for
// such channels: expires_at of others may be infinite.
const refreshInSQL = `CASE WHEN m.top_offset = 0 THEN
	EXTRACT(EPOCH FROM (m.expires_at - NOW()) - (m.expires_at - m.updated_at) / 2)::float8 END`

// emptyMetaKeeper keeps the meta of channels without publications while they
// have subscribers on this node, which is what HistoryMetaTTLUnpublished
// promises. In Redis position checks do it, but centrifuge skips them for a
// broker with ReliableDelivery. If the meta expired under a subscriber, the
// first publication would come with another epoch and be an insufficient state.
//
// It holds only those channels, with the time each is to be looked at, and
// queries only the ones which are due. This is safe as the expiry of such a
// meta never moves back: writes extend it and commit synchronously, and for
// the same reason reads don't shorten it.
type emptyMetaKeeper struct {
	hasSubscribers func(ch string) bool
	seed           maphash.Seed
	shards         [emptyMetaShards]emptyMetaShard
}

type emptyMetaShard struct {
	mu  sync.Mutex
	due map[string]int64 // Unix nanoseconds
}

func newEmptyMetaKeeper(hasSubscribers func(ch string) bool) *emptyMetaKeeper {
	k := &emptyMetaKeeper{hasSubscribers: hasSubscribers, seed: maphash.MakeSeed()}
	for i := range k.shards {
		k.shards[i].due = map[string]int64{}
	}
	return k
}

func (k *emptyMetaKeeper) shard(ch string) *emptyMetaShard {
	return &k.shards[maphash.String(k.seed, ch)%emptyMetaShards]
}

// watch is called for each read of an empty channel. Centrifuge adds a
// subscriber before it reads the position, so a subscriber's read is seen.
func (k *emptyMetaKeeper) watch(ch string, refreshIn float64, now time.Time) {
	if k.hasSubscribers(ch) {
		k.schedule(ch, refreshIn, now)
	}
}

func (k *emptyMetaKeeper) schedule(ch string, refreshIn float64, now time.Time) {
	wait := min(max(refreshIn, 0), emptyMetaMaxWait.Seconds())
	s := k.shard(ch)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.due[ch] = now.UnixNano() + int64(wait*float64(time.Second))
}

func (k *emptyMetaKeeper) forget(channels ...string) {
	for _, ch := range channels {
		s := k.shard(ch)
		s.mu.Lock()
		delete(s.due, ch)
		s.mu.Unlock()
	}
}

func (k *emptyMetaKeeper) dueChannels(now time.Time) []string {
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

// runEmptyMetaKeeper runs twice per position check delay: with the shortest
// TTL, 3 delays, one failed run still doesn't lose a meta.
func (e *PostgresStreamBroker) runEmptyMetaKeeper() {
	interval := e.node.Config().ClientChannelPositionCheckDelay / 2
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-e.cancelCtx.Done():
			return
		case <-ticker.C:
			e.keepEmptyMeta(e.cancelCtx, time.Now())
		}
	}
}

// keepEmptyMeta extends the due metas with less than half of their TTL left.
//
// SKIP LOCKED, as several nodes keep the same channels: a row locked by one of
// them, or by a publish, stays due and is looked at by the next run.
func (e *PostgresStreamBroker) keepEmptyMeta(ctx context.Context, now time.Time) {
	var channels []string
	for _, ch := range e.emptyMeta.dueChannels(now) {
		if e.emptyMeta.hasSubscribers(ch) {
			channels = append(channels, ch)
		} else {
			e.emptyMeta.forget(ch)
		}
	}
	if len(channels) == 0 {
		return
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
	`, e.names.meta, refreshInSQL)

	for batch := range slices.Chunk(channels, emptyMetaBatchSize) {
		if err := e.keepEmptyMetaBatch(ctx, query, batch, now); err != nil {
			if !isShutdownErr(err) {
				e.logErrorMsg("error keeping meta of empty channels", err)
			}
			return
		}
	}
}

func (e *PostgresStreamBroker) keepEmptyMetaBatch(ctx context.Context, query string, batch []string, now time.Time) error {
	rows, err := e.pool.Query(ctx, query, batch)
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
			e.emptyMeta.schedule(ch, refreshIn, now)
		} else {
			e.emptyMeta.forget(ch) // has publications, or no meta any more
		}
	}
	return nil
}
