package pgstreambroker

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/jackc/pgx/v5"
)

// History reads publications from a channel's history table, applying the
// per-channel TTL filter at read time. Unlike the Redis broker, the PG stream
// broker does NOT refresh meta TTL on read — meta TTL is refreshed on Publish
// (via the 3-tier MetaTTL fallback). This means channels with active readers
// but no publishers will eventually have their meta expire. This matches the
// map broker's ReadState/ReadStream behavior, and allows History to use read
// replicas for scaling.
//
// If the channel has no meta row (never published, or meta expired), History
// returns an empty result with a zero StreamPosition — matching the map
// broker's behavior on ErrNoRows.
func (e *PostgresStreamBroker) History(ch string, opts centrifuge.HistoryOptions) ([]*centrifuge.Publication, centrifuge.StreamPosition, error) {
	ctx := context.Background()
	pool := e.getReadPool(ch, true) // can use replicas — this is a pure read

	// Read meta — pure SELECT, no UPSERT.
	metaQuery := fmt.Sprintf(
		`SELECT top_offset, epoch, history_ttl, history_size FROM %s WHERE channel = $1`,
		e.names.meta,
	)

	var topOffset int64
	var epoch string
	var historyTTL *time.Duration
	var historySize *int

	err := pool.QueryRow(ctx, metaQuery, ch).Scan(&topOffset, &epoch, &historyTTL, &historySize)
	if errors.Is(err, pgx.ErrNoRows) {
		// Channel never published or meta expired — return empty, matching
		// map broker behavior. The centrifuge node treats empty epoch as
		// "don't check epoch" (epochOK := sinceEpoch == "" || sinceEpoch == streamTop.Epoch).
		return nil, centrifuge.StreamPosition{}, nil
	}
	if err != nil {
		return nil, centrifuge.StreamPosition{}, fmt.Errorf("postgres stream broker: history meta: %w", err)
	}

	streamPos := centrifuge.StreamPosition{
		Offset: uint64(topOffset),
		Epoch:  epoch,
	}

	// Filter.Limit == 0 → caller only wants the position.
	if opts.Filter.Limit == 0 {
		return nil, streamPos, nil
	}

	// Compute effective bounds Go-side.
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

	effectiveLimit := math.MaxInt32
	if opts.Filter.Limit > 0 && opts.Filter.Limit < effectiveLimit {
		effectiveLimit = opts.Filter.Limit
	}
	if historySize != nil && *historySize > 0 && *historySize < effectiveLimit {
		effectiveLimit = *historySize
	}

	// Read-time TTL filter cutoff.
	var cutoff time.Time
	if historyTTL != nil && *historyTTL > 0 {
		cutoff = time.Now().Add(-*historyTTL)
	} else {
		cutoff = time.Unix(0, 0) // sentinel — no filter
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
	`, e.names.stream, order)

	rows, err := pool.Query(ctx, rowsQuery, ch, effectiveStart, topOffset, cutoff, effectiveLimit)
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
		_ = raw[9] // prev_data — used by outbox, not by History
		pubs = append(pubs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, centrifuge.StreamPosition{}, fmt.Errorf("postgres stream broker: history rows: %w", err)
	}

	return pubs, streamPos, nil
}

// RemoveHistory wipes publications (kind=0) for a channel via the
// __PREFIX__remove_history SQL function.
func (e *PostgresStreamBroker) RemoveHistory(ch string) error {
	ctx := context.Background()
	query := fmt.Sprintf(`SELECT %s($1, $2)`, e.names.removeHistory)
	if _, err := e.pool.Exec(ctx, query, ch, e.conf.NumShards); err != nil {
		return fmt.Errorf("postgres stream broker: remove_history: %w", err)
	}
	return nil
}
