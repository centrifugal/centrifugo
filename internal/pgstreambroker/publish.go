package pgstreambroker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/centrifugal/centrifuge"
)

// Publish writes a publication to the history table and assigns a stream
// position. The publish SQL function does the heavy lifting (atomic shard
// lock + insert-or-lock meta + version/idempotency checks + offset increment
// + history INSERT + NOTIFY) — this Go-side method just marshals options,
// converts Go zero values to NULL pointers where the SQL function uses
// COALESCE, and reshapes the result.
func (e *PostgresStreamBroker) Publish(ch string, data []byte, opts centrifuge.PublishOptions) (centrifuge.PublishResult, error) {
	ctx := context.Background()

	// Marshal Tags to JSONB.
	var tagsJSON any
	if len(opts.Tags) > 0 {
		b, err := json.Marshal(opts.Tags)
		if err != nil {
			return centrifuge.PublishResult{}, fmt.Errorf("postgres stream broker: marshal tags: %w", err)
		}
		tagsJSON = json.RawMessage(b)
	}

	// HistoryTTL → INTERVAL pointer (nil if 0).
	var historyTTL *string
	if opts.HistoryTTL > 0 {
		s := durationToIntervalString(opts.HistoryTTL)
		historyTTL = &s
	}

	// HistorySize → INTEGER pointer (nil if 0).
	var historySize *int
	if opts.HistorySize > 0 {
		n := opts.HistorySize
		historySize = &n
	}

	// HistoryMetaTTL with 3-tier fallback: opts → node config → StreamRetention.
	// The third tier guarantees meta_expires_at is always set, so the meta
	// cleanup pass eventually fires for every channel.
	historyMetaTTL := opts.HistoryMetaTTL
	if historyMetaTTL == 0 {
		historyMetaTTL = e.node.Config().HistoryMetaTTL
	}
	if historyMetaTTL == 0 {
		historyMetaTTL = e.conf.StreamRetention
	}
	metaTTLStr := durationToIntervalString(historyMetaTTL)
	metaTTL := &metaTTLStr

	// Version handling — convert Go zero values to nil so the SQL function's
	// COALESCE preserves the existing meta value instead of overwriting with 0/''.
	var version *int64
	var versionEpoch *string
	if opts.Version > 0 {
		v := int64(opts.Version)
		version = &v
		if opts.VersionEpoch != "" {
			versionEpoch = &opts.VersionEpoch
		}
	}

	// Idempotency key — convert empty string to nil.
	var idempotencyKey *string
	var idempotencyTTL *string
	if opts.IdempotencyKey != "" {
		idempotencyKey = &opts.IdempotencyKey
		ttl := opts.IdempotentResultTTL
		if ttl == 0 {
			ttl = e.conf.IdempotentResultTTL
		}
		s := durationToIntervalString(ttl)
		idempotencyTTL = &s
	}

	// ClientInfo handling.
	var clientID, userID *string
	var connInfo, chanInfo []byte
	if opts.ClientInfo != nil {
		if opts.ClientInfo.ClientID != "" {
			clientID = &opts.ClientInfo.ClientID
		}
		if opts.ClientInfo.UserID != "" {
			userID = &opts.ClientInfo.UserID
		}
		connInfo = opts.ClientInfo.ConnInfo
		chanInfo = opts.ClientInfo.ChanInfo
	}

	// Key handling.
	var key *string
	if opts.Key != "" {
		key = &opts.Key
	}

	numShards := e.conf.NumShards
	skipShardLock := e.conf.SkipShardLock

	// Call __PREFIX__publish.
	query := fmt.Sprintf(`
		SELECT out_result_id, out_channel_offset, out_epoch, out_suppressed, out_suppress_reason
		FROM %s($1, $2, $3, $4, $5, $6, $7, $8, $9::interval, $10, $11::interval, $12, $13::interval, $14, $15, $16, $17, $18)
	`, e.names.publish)

	var resultID *int64
	var channelOffset int64
	var epoch string
	var suppressed bool
	var suppressReason *string

	err := e.pool.QueryRow(ctx, query,
		ch,
		e.dataParam(data),
		tagsJSON,
		clientID,
		userID,
		e.dataParam(connInfo),
		e.dataParam(chanInfo),
		key,
		historyTTL,
		historySize,
		metaTTL,
		idempotencyKey,
		idempotencyTTL,
		version,
		versionEpoch,
		opts.UseDelta,
		numShards,
		skipShardLock,
	).Scan(&resultID, &channelOffset, &epoch, &suppressed, &suppressReason)

	if err != nil {
		return centrifuge.PublishResult{}, fmt.Errorf("postgres stream broker: publish: %w", err)
	}

	result := centrifuge.PublishResult{
		StreamPosition: centrifuge.StreamPosition{
			Offset: uint64(channelOffset),
			Epoch:  epoch,
		},
	}
	if suppressed {
		result.Suppressed = true
		if suppressReason != nil {
			result.SuppressReason = centrifuge.SuppressReason(*suppressReason)
		}
	}

	// Contract quirk: for HistorySize<=0 || HistoryTTL<=0, the caller expects
	// an empty StreamPosition. The SQL function still wrote a row (so it's
	// delivered via the outbox) but we return zero per the Broker contract.
	if opts.HistorySize <= 0 || opts.HistoryTTL <= 0 {
		result.StreamPosition = centrifuge.StreamPosition{}
	}

	return result, nil
}

// PublishJoin publishes a join event. In fanout mode, short-circuits to the
// inner Broker (bypassing PG entirely). In non-fanout mode, inserts a kind=1
// row via the publish_join SQL function (which acquires the shard lock).
func (e *PostgresStreamBroker) PublishJoin(ch string, info *centrifuge.ClientInfo) error {
	if e.conf.Broker != nil {
		return e.conf.Broker.PublishJoin(ch, info)
	}
	return e.publishJoinViaHistory(ch, info)
}

// PublishLeave mirrors PublishJoin for leave events (kind=2).
func (e *PostgresStreamBroker) PublishLeave(ch string, info *centrifuge.ClientInfo) error {
	if e.conf.Broker != nil {
		return e.conf.Broker.PublishLeave(ch, info)
	}
	return e.publishLeaveViaHistory(ch, info)
}

func (e *PostgresStreamBroker) publishJoinViaHistory(ch string, info *centrifuge.ClientInfo) error {
	return e.publishJoinLeaveViaHistory(e.names.publishJoin, ch, info)
}

func (e *PostgresStreamBroker) publishLeaveViaHistory(ch string, info *centrifuge.ClientInfo) error {
	return e.publishJoinLeaveViaHistory(e.names.publishLeave, ch, info)
}

func (e *PostgresStreamBroker) publishJoinLeaveViaHistory(funcName, ch string, info *centrifuge.ClientInfo) error {
	ctx := context.Background()

	var clientID, userID string
	var connInfo, chanInfo []byte
	if info != nil {
		clientID = info.ClientID
		userID = info.UserID
		connInfo = info.ConnInfo
		chanInfo = info.ChanInfo
	}

	query := fmt.Sprintf(
		`SELECT %s($1, $2, $3, $4, $5, $6, $7)`,
		funcName,
	)
	var id int64
	if err := e.pool.QueryRow(ctx, query,
		ch,
		clientID,
		userID,
		e.dataParam(connInfo),
		e.dataParam(chanInfo),
		e.conf.NumShards,
		e.conf.SkipShardLock,
	).Scan(&id); err != nil {
		return fmt.Errorf("postgres stream broker: %s: %w", funcName, err)
	}
	return nil
}
