package pgstreambroker

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/pgoutbox"

	"github.com/centrifugal/centrifuge"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Kind values for the kind column on the history table.
const (
	kindPublication int16 = 0
	kindJoin        int16 = 1
	kindLeave       int16 = 2
)

// outboxWorkerConfig returns (pool, shardIDs) for outbox worker workerIdx.
// One shard per worker — the standard pgmapbroker pattern. With replicas,
// the worker reads from readPools[workerIdx % len(readPools)].
func (e *PostgresStreamBroker) outboxWorkerConfig(workerIdx int) (*pgxpool.Pool, []int) {
	pool := e.pool
	if len(e.readPools) > 0 {
		pool = e.readPools[workerIdx%len(e.readPools)]
	}
	return pool, []int{workerIdx}
}

// initOutboxCursor bootstraps the outbox worker cursor from MAX(id) of the
// history table. Any rows committed before the worker started are skipped
// (at-most-once live delivery; reachable via History recovery).
func (e *PostgresStreamBroker) initOutboxCursor(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var cursor int64
	err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT COALESCE(MAX(id), 0) FROM %s`, e.names.stream)).Scan(&cursor)
	return cursor, err
}

// runOutboxWorker is the per-shard outbox poller for the non-fanout case.
// Wraps pgoutbox.Worker; the broker-specific row scanning + dispatch lives
// in processOutboxBatch.
func (e *PostgresStreamBroker) runOutboxWorker(workerIdx int, initialCursor int64) {
	pool, shards := e.outboxWorkerConfig(workerIdx)

	w := &pgoutbox.Worker{
		Pool:         pool,
		ShardIDs:     shards,
		PollInterval: e.conf.Outbox.PollInterval,
		NotifyCh:     e.notifyCh,
		InitCursor: func(ctx context.Context, p *pgxpool.Pool) (int64, error) {
			return initialCursor, nil
		},
		ProcessBatch: func(ctx context.Context, p *pgxpool.Pool, cursor int64, sids []int) (int, int64, error) {
			return e.processOutboxBatch(ctx, p, cursor, sids)
		},
		ErrorFn: e.logErrorMsg,
	}
	w.Run(e.cancelCtx, e.closeCh)
}

// runOutboxWorkerWithLock is the per-shard outbox poller for fanout mode.
// Wraps pgoutbox.LockWorker which uses a session-level advisory lock so only
// one node per shard polls at a time.
//
// The initialCursor parameter is intentionally unused — pgoutbox.LockWorker
// calls InitCursor on every lock acquisition (not just at startup) so the
// cursor can catch up to MAX(id) after another node held the lock. Returning
// a fixed startup value here is not a correctness bug (subscribers dedup by
// offset; the position-check loop + history recovery close any live gap) but
// it costs O(rows_since_startup) of re-fanout work on every re-acquisition.
// Querying fresh MAX(id) each time keeps the steady-state cost bounded.
func (e *PostgresStreamBroker) runOutboxWorkerWithLock(workerIdx int, _ int64) {
	pollPool, shards := e.outboxWorkerConfig(workerIdx)

	lw := &pgoutbox.LockWorker{
		LockPool:      e.pool,
		PollPool:      pollPool,
		ShardIDs:      shards,
		LockID:        e.conf.Outbox.AdvisoryLockBaseID + int64(workerIdx),
		PollInterval:  e.conf.Outbox.PollInterval,
		RetryInterval: e.conf.Outbox.AdvisoryLockRetryInterval,
		InitCursor:    e.initOutboxCursor,
		ProcessBatch: func(ctx context.Context, p *pgxpool.Pool, cursor int64, sids []int) (int, int64, error) {
			return e.processOutboxBatch(ctx, p, cursor, sids)
		},
		ErrorFn: func(msg string, err error) { e.logErrorMsg(msg, err) },
		InfoFn:  func(msg string) { /* TODO: info logging */ },
	}
	lw.Run(e.cancelCtx, e.closeCh)
}

// runNotificationListener listens for pg_notify on the broker's notify channel
// and wakes the outbox worker(s). Thin wrapper around pgoutbox.NotificationListener.
func (e *PostgresStreamBroker) runNotificationListener() {
	pool := e.pool
	if e.notifyPool != nil {
		pool = e.notifyPool
	}
	l := &pgoutbox.NotificationListener{
		Pool:     pool,
		Channel:  e.names.notifyChannel,
		NotifyCh: e.notifyCh,
		ErrorFn:  e.logErrorMsg,
		OnReady:  func() { e.notifyListenerReady.Store(true) },
	}
	l.Run(e.cancelCtx, e.closeCh)
}

// processOutboxBatch reads up to BatchSize history rows past the cursor for
// the given shard, scans them with zero-allocation pgx helpers, and dispatches
// each one based on its kind:
//
//   - kind=0 (publication): construct *Publication, call HandlePublication
//     (or fanout's inner Broker.Publish in fanout mode)
//   - kind=1 (join): construct *ClientInfo, call HandleJoin
//   - kind=2 (leave): construct *ClientInfo, call HandleLeave
//
// Returns (rowsProcessed, newCursor, error).
func (e *PostgresStreamBroker) processOutboxBatch(
	ctx context.Context, pool *pgxpool.Pool,
	cursor int64, shardIDs []int,
) (int, int64, error) {
	query := fmt.Sprintf(`
		SELECT id, shard_id, channel, channel_offset, epoch, kind, data, tags,
		       client_id, user_id, conn_info, chan_info, prev_data
		  FROM %s
		 WHERE id > $1 AND shard_id = ANY($2)
		 ORDER BY id
		 LIMIT $3
	`, e.names.stream)

	rows, err := pool.Query(ctx, query, cursor, shardIDs, e.conf.Outbox.BatchSize)
	if err != nil {
		return 0, cursor, fmt.Errorf("postgres stream broker: outbox query: %w", err)
	}
	defer rows.Close()

	allocHint := e.conf.Outbox.BatchSize
	if allocHint > 1001 {
		allocHint = 1001
	}
	arena := byteArena{buf: make([]byte, 0, allocHint*64)}
	pubBacking := make([]centrifuge.Publication, 0, allocHint)
	infoBacking := make([]centrifuge.ClientInfo, 0, allocHint)

	type batchEntry struct {
		id      int64
		channel string
		kind    int16
		pub     *centrifuge.Publication
		info    *centrifuge.ClientInfo
		epoch   string
	}
	entries := make([]batchEntry, 0, allocHint)

	var fmts pgColFormats
	maxID := cursor
	for rows.Next() {
		if fmts == nil {
			fmts = pgColFormatsFromRows(rows)
		}
		raw := rows.RawValues()
		// Column order:
		// 0 id, 1 shard_id, 2 channel, 3 channel_offset, 4 epoch, 5 kind,
		// 6 data, 7 tags, 8 client_id, 9 user_id, 10 conn_info, 11 chan_info,
		// 12 prev_data
		id := pgRawInt64(raw[0], fmts[0])
		channel := pgRawString(&arena, raw[2])
		channelOffset := pgRawUint64(raw[3], fmts[3])
		epoch := pgRawString(&arena, raw[4])
		kind := pgRawInt16(raw[5], fmts[5])

		entry := batchEntry{
			id:      id,
			channel: channel,
			kind:    kind,
			epoch:   epoch,
		}

		switch kind {
		case kindPublication:
			pubBacking = append(pubBacking, centrifuge.Publication{})
			p := &pubBacking[len(pubBacking)-1]
			p.Offset = channelOffset
			p.Epoch = epoch
			p.Data = e.rawDataBytes(&arena, raw[6], fmts[6])
			p.Tags = pgRawJSONBMap(raw[7])
			if raw[8] != nil {
				infoBacking = append(infoBacking, centrifuge.ClientInfo{
					ClientID: pgRawString(&arena, raw[8]),
					UserID:   pgRawString(&arena, raw[9]),
					ConnInfo: e.rawDataBytes(&arena, raw[10], fmts[10]),
					ChanInfo: e.rawDataBytes(&arena, raw[11], fmts[11]),
				})
				p.Info = &infoBacking[len(infoBacking)-1]
			}
			entry.pub = p
		case kindJoin, kindLeave:
			if raw[8] != nil {
				infoBacking = append(infoBacking, centrifuge.ClientInfo{
					ClientID: pgRawString(&arena, raw[8]),
					UserID:   pgRawString(&arena, raw[9]),
					ConnInfo: e.rawDataBytes(&arena, raw[10], fmts[10]),
					ChanInfo: e.rawDataBytes(&arena, raw[11], fmts[11]),
				})
				entry.info = &infoBacking[len(infoBacking)-1]
			}
		}

		entries = append(entries, entry)
		if id > maxID {
			maxID = id
		}
	}
	if err := rows.Err(); err != nil {
		return 0, cursor, fmt.Errorf("postgres stream broker: outbox rows: %w", err)
	}
	rows.Close()

	// Dispatch entries.
	for _, en := range entries {
		switch en.kind {
		case kindPublication:
			sp := centrifuge.StreamPosition{Offset: en.pub.Offset, Epoch: en.epoch}
			if e.conf.Broker != nil {
				// Fanout mode: forward to inner broker.
				_, err := e.conf.Broker.Publish(en.channel, en.pub.Data, centrifuge.PublishOptions{
					ClientInfo: en.pub.Info,
					Tags:       en.pub.Tags,
					Offset:     en.pub.Offset,
					Epoch:      en.epoch,
				})
				if err != nil {
					e.logErrorMsg("outbox forward to inner broker", err)
				}
			} else {
				if err := e.eventHandler.HandlePublication(en.channel, en.pub, sp, false, nil); err != nil {
					e.logErrorMsg("outbox HandlePublication", err)
				}
			}
		case kindJoin:
			if e.conf.Broker != nil {
				// Joins shouldn't reach here in fanout mode (PublishJoin
				// short-circuits to the inner broker), but handle defensively.
				if err := e.conf.Broker.PublishJoin(en.channel, en.info); err != nil {
					e.logErrorMsg("outbox forward join to inner broker", err)
				}
			} else if en.info != nil {
				if err := e.eventHandler.HandleJoin(en.channel, en.info); err != nil {
					e.logErrorMsg("outbox HandleJoin", err)
				}
			}
		case kindLeave:
			if e.conf.Broker != nil {
				if err := e.conf.Broker.PublishLeave(en.channel, en.info); err != nil {
					e.logErrorMsg("outbox forward leave to inner broker", err)
				}
			} else if en.info != nil {
				if err := e.eventHandler.HandleLeave(en.channel, en.info); err != nil {
					e.logErrorMsg("outbox HandleLeave", err)
				}
			}
		}
	}

	// Update sampler cursor for the lag metric. With one shard per worker
	// the shardIDs slice has exactly one element.
	if e.sampler != nil && len(shardIDs) == 1 {
		e.sampler.storeCursor(shardIDs[0], maxID)
	}

	return len(entries), maxID, nil
}
