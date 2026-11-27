package pgstreambroker

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/pgemptymeta"

	"github.com/centrifugal/centrifuge"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// History reads publications from a channel's history table, applying the
// per-channel TTL filter at read time.
//
// A channel with publications is a pure read, which can use replicas. Its meta
// TTL is refreshed on Publish only, unlike in the Redis broker.
//
// A channel without publications gets an epoch, as in the memory and Redis
// brokers, see writeEmptyMeta: one write on the primary, repeated when less
// than half of its TTL (see emptyMetaTTL) is left. A replica without the meta
// row sends the read to the primary.
func (e *PostgresStreamBroker) History(ch string, opts centrifuge.HistoryOptions) ([]*centrifuge.Publication, centrifuge.StreamPosition, error) {
	ctx := context.Background()
	initial := e.initialMetaTTL(opts.MetaTTLInitial)
	metaTTL := e.emptyMetaTTL(opts.MetaTTL, initial)

	pool := e.getReadPool(ch, true) // can use replicas
	meta, err := e.readMeta(ctx, pool, ch, metaTTL)
	if err != nil {
		return nil, centrifuge.StreamPosition{}, err
	}
	if meta.needsWrite && pool != e.pool {
		// The replica may lag: the primary decides, and the publications have
		// to be read where the position was.
		pool = e.pool
		if meta, err = e.readMeta(ctx, pool, ch, metaTTL); err != nil {
			return nil, centrifuge.StreamPosition{}, err
		}
	}
	if meta.needsWrite {
		if meta, err = e.writeEmptyMeta(ctx, ch, metaTTL, meta.epoch == ""); err != nil {
			return nil, centrifuge.StreamPosition{}, err
		}
	}
	if meta.refreshIn != nil && initial > 0 {
		e.emptyMeta.Get().Watch(ch, *meta.refreshIn, time.Now())
	}
	topOffset, epoch := meta.topOffset, meta.epoch
	historyTTL, historySize := meta.historyTTL, meta.historySize

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
	// Compute the offset window [effectiveStart, effectiveEnd]. Forward returns
	// publications after Since (ascending); reverse returns publications before
	// Since (descending) - the boundary sits on the opposite side of the cursor,
	// as in the memory/redis/pgmapbroker brokers. Using Since+1 for both
	// directions made reverse pagination return already-seen or empty pages and
	// never reach older messages.
	effectiveStart := windowStart
	effectiveEnd := topOffset
	if opts.Filter.Reverse {
		// Reverse returns publications strictly before Since (descending).
		// Guard sinceOffset == 0: centrifuge's Since.Offset is uint64, so its
		// `since.Offset - 1` underflows to MaxUint64 for offset 0, which means
		// "read the whole stream from the top". We keep effectiveEnd = topOffset
		// in that case instead of computing -1 (which would match no rows).
		if opts.Filter.Since != nil && sinceOffset > 0 {
			effectiveEnd = sinceOffset - 1
		}
	} else {
		effectiveStart = sinceOffset + 1
		if windowStart > effectiveStart {
			effectiveStart = windowStart
		}
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

	// The `epoch = $5` predicate rejects rows from prior epochs that linger
	// between meta TTL expiry and the next partition retention drop. The
	// time-based `created_at > cutoff` filter usually catches them too, but
	// fails when history_ttl is increased between the dead epoch's last
	// publish and the read — the new wider cutoff lets old rows through.
	// The new (channel, epoch, channel_offset) WHERE kind=0 partial index
	// resolves the predicate without scanning dead-epoch rows.
	rowsQuery := fmt.Sprintf(`
		SELECT channel_offset, epoch, data, tags, client_id, user_id, conn_info, chan_info, prev_data
		  FROM %s
		 WHERE channel = $1
		   AND kind = 0
		   AND channel_offset >= $2
		   AND channel_offset <= $3
		   AND created_at > $4
		   AND epoch = $5
		 ORDER BY channel_offset %s
		 LIMIT $6
	`, e.names.stream, order)

	rows, err := pool.Query(ctx, rowsQuery, ch, effectiveStart, effectiveEnd, cutoff, epoch, effectiveLimit)
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
		_ = raw[8] // prev_data — used by outbox, not by History
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

type streamMeta struct {
	topOffset   int64
	epoch       string
	historyTTL  *time.Duration
	historySize *int
	// needsWrite is set when the channel has no meta row, or has no
	// publications and less than half of its TTL left.
	needsWrite bool
	// refreshIn is set for a channel without publications, see pgemptymeta.RefreshInSQL.
	refreshIn *float64
}

// initialMetaTTL is the initial meta TTL of a read: the subscription's
// (HistoryOptions.MetaTTLInitial, from its namespace), or the global one, see
// pgemptymeta.Initial.
func (e *PostgresStreamBroker) initialMetaTTL(requested time.Duration) time.Duration {
	return pgemptymeta.Initial(requested, e.node.Config().HistoryMetaTTLInitial)
}

// emptyMetaTTL is how long the meta of a channel without publications is
// kept: at least StreamRetention without an initial TTL, as before the
// option, see pgemptymeta.TTL with it.
func (e *PostgresStreamBroker) emptyMetaTTL(requested, initial time.Duration) string {
	if initial <= 0 {
		return durationToIntervalString(max(e.historyMetaTTL(requested), e.conf.StreamRetention))
	}
	return durationToIntervalString(pgemptymeta.TTL(e.historyMetaTTL(requested), initial, e.node.Config().ClientChannelPositionCheckDelay))
}

func (e *PostgresStreamBroker) readMeta(ctx context.Context, pool *pgxpool.Pool, ch, metaTTL string) (streamMeta, error) {
	query := fmt.Sprintf(`
		SELECT top_offset, epoch, history_ttl, history_size,
		       top_offset = 0 AND expires_at < NOW() + $2::interval / 2,
		       %s
		  FROM %s AS m
		 WHERE channel = $1
	`, pgemptymeta.RefreshInSQL, e.names.meta)

	var m streamMeta
	err := pool.QueryRow(ctx, query, ch, metaTTL).Scan(&m.topOffset, &m.epoch, &m.historyTTL, &m.historySize, &m.needsWrite, &m.refreshIn)
	if errors.Is(err, pgx.ErrNoRows) {
		return streamMeta{needsWrite: true}, nil
	}
	if err != nil {
		return streamMeta{}, fmt.Errorf("postgres stream broker: history meta: %w", err)
	}
	return m, nil
}

// writeEmptyMeta gives a channel without publications an epoch, or extends the
// TTL of the one it has. A position with an empty epoch would match any later
// stream, so a client could resume over a stream which was created and
// forgotten while it was away, and be told that it recovered.
//
// A create does not wait for the WAL flush or for synchronous standbys: if
// the row is lost in a crash, the next one gets another epoch and positions in
// the lost one are unrecoverable, which is safe. A publish on top of the row
// commits synchronously and so makes it durable too. An extension commits
// synchronously, see pgemptymeta.Keeper. The setting is local to this statement's
// transaction.
func (e *PostgresStreamBroker) writeEmptyMeta(ctx context.Context, ch, metaTTL string, create bool) (streamMeta, error) {
	query := fmt.Sprintf(`
		WITH commit_mode AS MATERIALIZED (
			SELECT set_config('synchronous_commit',
				CASE WHEN $3 THEN 'off' ELSE current_setting('synchronous_commit') END, true) AS value
		)
		INSERT INTO %s AS m (channel, epoch, expires_at, updated_at)
		SELECT $1, substr(md5(random()::text || random()::text), 1, 8), NOW() + $2::interval, NOW()
		  FROM commit_mode
		 WHERE commit_mode.value IS NOT NULL
		ON CONFLICT (channel) DO UPDATE
		   SET expires_at = CASE WHEN m.top_offset = 0 AND m.expires_at < EXCLUDED.expires_at
		                         THEN EXCLUDED.expires_at ELSE m.expires_at END,
		       updated_at = CASE WHEN m.top_offset = 0 AND m.expires_at < EXCLUDED.expires_at
		                         THEN EXCLUDED.updated_at ELSE m.updated_at END
		RETURNING m.top_offset, m.epoch, m.history_ttl, m.history_size, %s
	`, e.names.meta, pgemptymeta.RefreshInSQL)

	var m streamMeta
	err := e.pool.QueryRow(ctx, query, ch, metaTTL, create).Scan(&m.topOffset, &m.epoch, &m.historyTTL, &m.historySize, &m.refreshIn)
	if err != nil {
		return streamMeta{}, fmt.Errorf("postgres stream broker: write empty channel meta: %w", err)
	}
	return m, nil
}
