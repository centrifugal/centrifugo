package pgstreambroker

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/jackc/pgx/v5"
)

// History reads publications from a channel's history table, applying the
// per-channel TTL filter at read time. Matches Redis broker semantics:
//   - Creates the meta row if missing (with a fresh epoch).
//   - Refreshes meta_expires_at on every read using the effective MetaTTL.
//   - Returns rows bounded by HistorySize (clamped at read time so even a
//     forward query with NoLimit on a tight-HistorySize channel returns the
//     most-recent-N window, matching MAXLEN-trim semantics).
//
// Runs against the primary pool (not replicas) because of the meta UPSERT.
func (e *PostgresStreamBroker) History(ch string, opts centrifuge.HistoryOptions) ([]*centrifuge.Publication, centrifuge.StreamPosition, error) {
	ctx := context.Background()

	// Resolve effective MetaTTL via fallback chain.
	historyMetaTTL := opts.MetaTTL
	if historyMetaTTL == 0 {
		historyMetaTTL = e.node.Config().HistoryMetaTTL
	}
	if historyMetaTTL == 0 {
		historyMetaTTL = e.conf.StreamRetention
	}
	metaTTLStr := durationToIntervalString(historyMetaTTL)

	// Open a READ COMMITTED transaction. The UPSERT acquires the meta row
	// lock atomically and returns the post-update state. The subsequent
	// rows query uses the returned top_offset as a consistency anchor
	// (channel_offset <= top_offset) so concurrent publishes don't leak
	// rows beyond what we already returned.
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return nil, centrifuge.StreamPosition{}, fmt.Errorf("postgres stream broker: history begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	upsert := fmt.Sprintf(`
		INSERT INTO %s (channel, top_offset, epoch, meta_expires_at, updated_at)
		VALUES ($1, 0, substr(md5(random()::text || random()::text), 1, 8),
		        NOW() + $2::interval, NOW())
		ON CONFLICT (channel) DO UPDATE
		   SET meta_expires_at = NOW() + $2::interval,
		       updated_at = NOW()
		RETURNING top_offset, epoch, history_ttl, history_size
	`, e.names.meta)

	var topOffset int64
	var epoch string
	var historyTTL *time.Duration
	var historySize *int

	// Use *time.Duration scan via interval handling — pgx maps interval to
	// time.Duration through pgtype/interval (microsecond precision).
	var historyTTLRaw pgx.Row
	_ = historyTTLRaw

	row := tx.QueryRow(ctx, upsert, ch, metaTTLStr)
	if err := row.Scan(&topOffset, &epoch, &historyTTL, &historySize); err != nil {
		return nil, centrifuge.StreamPosition{}, fmt.Errorf("postgres stream broker: history meta upsert: %w", err)
	}

	streamPos := centrifuge.StreamPosition{
		Offset: uint64(topOffset),
		Epoch:  epoch,
	}

	// Filter.Limit == 0 → caller only wants the position. Skip the rows query.
	if opts.Filter.Limit == 0 {
		_ = tx.Commit(ctx)
		return nil, streamPos, nil
	}

	// Compute effective bounds Go-side.
	//
	// window_start: lower bound on channel_offset, clamped to history_size
	// so a too-old Since position pivots to the most-recent-N window.
	windowStart := int64(1)
	if historySize != nil && *historySize > 0 {
		if topOffset >= int64(*historySize) {
			windowStart = topOffset - int64(*historySize) + 1
		}
	}
	sinceOffset := int64(0)
	if opts.Filter.Since != nil {
		sinceOffset = int64(opts.Filter.Since.Offset)
	}
	effectiveStart := sinceOffset + 1
	if windowStart > effectiveStart {
		effectiveStart = windowStart
	}

	// effective_limit: clamped to min(filter.Limit, history_size).
	effectiveLimit := math.MaxInt32
	if opts.Filter.Limit > 0 && opts.Filter.Limit < effectiveLimit {
		effectiveLimit = opts.Filter.Limit
	}
	if historySize != nil && *historySize > 0 && *historySize < effectiveLimit {
		effectiveLimit = *historySize
	}

	// cutoff: read-time TTL filter timestamp.
	var cutoff time.Time
	if historyTTL != nil && *historyTTL > 0 {
		cutoff = time.Now().Add(-*historyTTL)
	} else {
		// Sentinel "no filter" — use Unix zero, which is well before any row.
		cutoff = time.Unix(0, 0)
	}

	order := "ASC"
	if opts.Filter.Reverse {
		order = "DESC"
	}

	rowsQuery := fmt.Sprintf(`
		SELECT channel_offset, epoch, data, tags, client_id, user_id, conn_info, chan_info, key, prev_data
		  FROM %s
		 WHERE channel = $1
		   AND kind = 0
		   AND channel_offset >= $2
		   AND channel_offset <= $3
		   AND created_at > $4
		 ORDER BY channel_offset %s
		 LIMIT $5
	`, e.names.history, order)

	rows, err := tx.Query(ctx, rowsQuery, ch, effectiveStart, topOffset, cutoff, effectiveLimit)
	if err != nil {
		return nil, centrifuge.StreamPosition{}, fmt.Errorf("postgres stream broker: history query: %w", err)
	}
	defer rows.Close()

	allocHint := effectiveLimit
	if allocHint > 1024 {
		allocHint = 1024
	}
	arena := byteArena{buf: make([]byte, 0, allocHint*64)}
	backing := make([]centrifuge.Publication, 0, allocHint)
	pubs := make([]*centrifuge.Publication, 0, allocHint)

	var fmts pgColFormats
	for rows.Next() {
		if fmts == nil {
			fmts = pgColFormatsFromRows(rows)
		}
		raw := rows.RawValues()
		// Column order:
		// 0 channel_offset, 1 epoch, 2 data, 3 tags, 4 client_id,
		// 5 user_id, 6 conn_info, 7 chan_info, 8 key, 9 prev_data
		backing = append(backing, centrifuge.Publication{})
		p := &backing[len(backing)-1]
		p.Offset = pgRawUint64(raw[0], fmts[0])
		p.Epoch = pgRawString(&arena, raw[1])
		p.Data = e.rawDataBytes(&arena, raw[2], fmts[2])
		p.Tags = pgRawJSONBMap(raw[3])
		if raw[4] != nil {
			p.Info = &centrifuge.ClientInfo{
				ClientID: pgRawString(&arena, raw[4]),
				UserID:   pgRawString(&arena, raw[5]),
				ConnInfo: e.rawDataBytes(&arena, raw[6], fmts[6]),
				ChanInfo: e.rawDataBytes(&arena, raw[7], fmts[7]),
			}
		}
		p.Key = pgRawString(&arena, raw[8])
		// raw[9] (prev_data) is read but not stored on Publication directly —
		// it's used by the outbox path for delta computation, not by History.
		_ = raw[9]
		pubs = append(pubs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, centrifuge.StreamPosition{}, fmt.Errorf("postgres stream broker: history rows: %w", err)
	}
	rows.Close()

	if err := tx.Commit(ctx); err != nil {
		return nil, centrifuge.StreamPosition{}, fmt.Errorf("postgres stream broker: history commit: %w", err)
	}

	return pubs, streamPos, nil
}

// RemoveHistory wipes publications (kind=0) for a channel via the
// __PREFIX__remove_history SQL function. The function acquires the per-shard
// lock so that any in-progress publish either commits before the DELETE or
// runs after.
func (e *PostgresStreamBroker) RemoveHistory(ch string) error {
	ctx := context.Background()
	query := fmt.Sprintf(`SELECT %s($1, $2, $3)`, e.names.removeHistory)
	if _, err := e.pool.Exec(ctx, query, ch, e.conf.NumShards, e.conf.SkipShardLock); err != nil {
		return fmt.Errorf("postgres stream broker: remove_history: %w", err)
	}
	return nil
}
