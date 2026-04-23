// Package pgstreambroker implements centrifuge.Broker using PostgreSQL as
// the backing store for stream subscriptions. It provides:
//
//   - Transactional publishing: applications can publish in the same SQL
//     transaction as their business writes, eliminating the dual-write problem.
//   - Per-channel HistoryTTL and HistorySize honored at read time via the
//     two-TTL meta model (matches Redis broker semantics).
//   - At-most-once live delivery via the polling outbox; reliable recovery
//     via History() reads.
//   - Always-partitioned schema with daily partition rolling for vacuum-free
//     bulk cleanup. PartitionRetentionDays defaults to 7 days; the partition
//     retention worker (pgoutbox.Partitioner) drops old partitions whole.
//
// PG version requirement: PostgreSQL 13 or later (for partitioned table
// features and INSERT ... ON CONFLICT DO UPDATE on partitioned tables).
//
// See internal/pgstreambroker/internal/sql/schema.sql for the underlying
// schema and SQL functions.
package pgstreambroker

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

//go:embed internal/sql/schema.sql
var postgresSchemaTemplate string

// schemaVersion is the current schema version. Bump when adding migrations.
var schemaVersion = 1

// schemaMigrations maps target version to migration SQL. Each migration must
// handle BOTH prefixes (jsonb + binary) and be idempotent. Empty for v1.
var schemaMigrations = map[int]string{}

// renderSchema substitutes placeholders in the embedded schema template.
//
//	__PREFIX__       → e.g. "cf_stream_" (includes trailing underscore)
//	__DATA_TYPE__    → "JSONB" or "BYTEA"
//	__STREAM_TABLE__ → e.g. "cf_stream" (prefix without trailing underscore)
func renderSchema(prefix string, binary bool) string {
	dataType := "JSONB"
	if binary {
		dataType = "BYTEA"
	}
	streamTable := strings.TrimRight(prefix, "_")
	return strings.NewReplacer(
		"__STREAM_TABLE__", streamTable,
		"__PREFIX__", prefix,
		"__DATA_TYPE__", dataType,
	).Replace(postgresSchemaTemplate)
}

// splitSchemaSQL splits the schema SQL into DDL (tables+indexes) and function
// definitions. They must be executed as separate transactions to avoid deadlocks
// with concurrent function executions during rolling deploys.
func splitSchemaSQL(sql string) (ddl, funcs string) {
	const marker = "CREATE OR REPLACE FUNCTION"
	i := strings.Index(sql, marker)
	if i < 0 {
		return sql, ""
	}
	return sql[:i], sql[i:]
}

// SchemaObject identifies a database object referenced in a SchemaError.
type SchemaObject struct {
	Type string // "table", "function", "index", "schema"
	Name string
}

// SchemaError wraps a schema-related error with object and operation info.
type SchemaError struct {
	Object SchemaObject
	Op     string // "create", "verify", etc.
	Err    error
}

func (e *SchemaError) Error() string {
	return fmt.Sprintf("postgres stream broker: %s %s %q: %v", e.Op, e.Object.Type, e.Object.Name, e.Err)
}

func (e *SchemaError) Unwrap() error {
	return e.Err
}

// pgNames holds precomputed table/function/notify-channel names. The
// jsonbPrefix and binaryPrefix are kept so EnsureSchema can iterate both
// variants regardless of the active BinaryData mode.
//
//	jsonbPrefix  = userPrefix + "_stream_"          (e.g. "cf_stream_")
//	binaryPrefix = userPrefix + "_binary_stream_"   (e.g. "cf_binary_stream_")
//
// The history/meta/idempotency/... fields are computed from the *active*
// prefix (jsonbPrefix when BinaryData is false, binaryPrefix when true).
type pgNames struct {
	stream, meta, idempotency, shardLock, schemaVersion string // table names (active variant)
	publish, publishStrict, publishJoin, publishLeave   string // function names (active variant)
	removeHistory                                       string
	notifyChannel                                       string

	jsonbPrefix, binaryPrefix string
}

func newPgNames(userPrefix string, binary bool) pgNames {
	jsonbPrefix := userPrefix + "_stream_"
	binaryPrefix := userPrefix + "_binary_stream_"

	p := jsonbPrefix
	if binary {
		p = binaryPrefix
	}
	return pgNames{
		stream:        strings.TrimRight(p, "_"), // cf_stream (not cf_stream_history)
		meta:          p + "meta",
		idempotency:   p + "idempotency",
		shardLock:     p + "shard_lock",
		schemaVersion: p + "schema_version",
		publish:       p + "publish",
		publishStrict: p + "publish_strict",
		publishJoin:   p + "publish_join",
		publishLeave:  p + "publish_leave",
		removeHistory: p + "remove_history",
		notifyChannel: p + "notify",
		jsonbPrefix:   jsonbPrefix,
		binaryPrefix:  binaryPrefix,
	}
}

// durationToIntervalString converts a time.Duration to a PostgreSQL interval string.
func durationToIntervalString(d time.Duration) string {
	ms := d.Milliseconds()
	if ms > 0 {
		return strconv.FormatInt(ms, 10) + " milliseconds"
	}
	return "1 milliseconds"
}

// hashtext approximates PostgreSQL hashtext() for shard routing. We only need
// consistency within a process, not cross-process compatibility with PG.
func hashtext(s string) int32 {
	var h int32
	for i := 0; i < len(s); i++ {
		h = h*31 + int32(s[i])
	}
	return h
}

func abs32(n int32) int {
	if n < 0 {
		return int(-n)
	}
	return int(n)
}

// OutboxConfig configures outbox-based delivery.
type OutboxConfig struct {
	// PollInterval is how often to poll for new history rows when idle.
	// Default: 50ms.
	PollInterval time.Duration

	// BatchSize is the maximum number of rows to process per batch.
	// Default: 1000.
	BatchSize int

	// AdvisoryLockBaseID is the base ID for PostgreSQL advisory locks used to
	// claim shards when Broker fan-out is enabled. Lock ID = AdvisoryLockBaseID + shardID.
	// Default: 726966531 (one above the map broker base to avoid collision).
	AdvisoryLockBaseID int64

	// AdvisoryLockRetryInterval is how often to retry advisory lock acquisition
	// when Broker fan-out is enabled. Default: 5s.
	AdvisoryLockRetryInterval time.Duration
}

// PostgresStreamBrokerConfig configures the PostgreSQL stream broker.
type PostgresStreamBrokerConfig struct {
	// Name is an observability label included in metrics and logs.
	Name string

	// DSN is the primary PostgreSQL connection string for writes.
	DSN string

	// TLS is an optional TLS configuration applied to all pools (primary,
	// replicas, notify). Use instead of embedding TLS params in the DSN.
	TLS configtypes.TLSConfig

	// PoolSize sets the maximum number of connections in the primary pool.
	// Default: 32.
	PoolSize int

	// NumShards is the total number of shards for parallel delivery workers.
	// Channels are distributed via hash(channel) % NumShards.
	// Default: 16.
	NumShards int

	// CleanupInterval is how often the cleanup worker and partition worker tick.
	// Default: 1m.
	CleanupInterval time.Duration

	// IdempotentResultTTL is the default TTL for idempotency cache entries.
	// Default: 5m.
	IdempotentResultTTL time.Duration

	// Outbox configures the outbox-based delivery mode.
	Outbox OutboxConfig

	// BinaryData uses BYTEA columns instead of JSONB for data fields.
	// Set true if data payloads are not valid JSON (binary/protobuf).
	BinaryData bool

	// TablePrefix is the user-facing namespace prefix. Default "cf".
	// The broker appends "_stream_" or "_binary_stream_" internally, so
	// the default produces:
	//
	//   cf_stream_history, cf_stream_meta, cf_stream_idempotency, cf_stream_publish, ...
	//
	// and their cf_binary_stream_* counterparts for the binary variant.
	TablePrefix string

	// StreamRetention is the safety floor for HistoryMetaTTL when neither
	// PublishOptions nor node config sets it. Default: 24h. Guarantees that
	// every channel meta row eventually expires (no NULL expires_at).
	StreamRetention time.Duration

	// UseNotify enables LISTEN/NOTIFY for low-latency outbox wakeup.
	// When false (default), outbox worker uses PollInterval-based polling only.
	UseNotify bool

	// NotifyDSN is an optional separate connection string used exclusively for
	// the LISTEN connection when UseNotify is true. Set this to a direct
	// PostgreSQL URL (bypassing PGBouncer) when DSN points at a PGBouncer
	// endpoint — PGBouncer transaction pooling mode is incompatible with
	// LISTEN/NOTIFY. If empty, the primary DSN pool is used (fine for direct
	// PostgreSQL connections).
	NotifyDSN string

	// FineGrainedHistoryCleanup enables an opt-in chunked DELETE pass that
	// removes history rows past their channel's history_ttl, instead of
	// waiting for partition retention. Use this for tight-storage deployments
	// where HistoryTTL is much smaller than PartitionRetentionDays. Default: false
	// (rows live up to PartitionRetentionDays; the read-time TTL filter
	// guarantees correctness regardless).
	FineGrainedHistoryCleanup bool

	// CleanupBatchSize bounds each fine-grained cleanup DELETE to this many
	// rows per transaction. Default: 1000. Only used when
	// FineGrainedHistoryCleanup is true.
	CleanupBatchSize int

	// CleanupChunkPause is the pause between fine-grained cleanup chunks,
	// giving autovacuum room between batches. Default: 100ms. Only used
	// when FineGrainedHistoryCleanup is true.
	CleanupChunkPause time.Duration

	// ReplicaDSN is a list of read-replica connection strings. When set,
	// outbox cursor bootstraps go to the replica routing instead of the
	// primary. Default: empty (all reads go to primary).
	ReplicaDSN []string

	// ReplicaPoolSize sets max connections per replica pool.
	// Default: same as PoolSize.
	ReplicaPoolSize int

	// Broker is an optional Broker (e.g. RedisBroker) for PUB/SUB fan-out.
	// When set, outbox workers use advisory locks (one worker per shard
	// across all nodes) and publish via the inner Broker instead of
	// HandlePublication directly. PublishJoin/PublishLeave bypass PG
	// entirely and go through the inner broker.
	Broker centrifuge.Broker

	// PartitionLookaheadDays controls how many future daily partitions to
	// pre-create. Required > 0 so writes don't fail at the day rollover.
	// Default: 2 (gives a 48-hour safety window).
	PartitionLookaheadDays int

	// PartitionRetentionDays controls how old a partition can be before
	// it gets dropped whole by the partition retention worker.
	//
	// Set to 0 for unlimited retention (the pgoutbox.Partitioner guard
	// treats RetentionDays <= 0 as "never drop"; old partitions accumulate).
	//
	// The OSS configtypes default is 7 (via the struct-tag default). Direct
	// Go-level construction (e.g. tests) must set this explicitly — there
	// is no implicit default in setDefaults so that 0 (unlimited) survives.
	PartitionRetentionDays int
}

func (c *PostgresStreamBrokerConfig) setDefaults() {
	if c.PoolSize <= 0 {
		c.PoolSize = 16
	}
	if c.NumShards <= 0 {
		c.NumShards = 16
	}
	if c.CleanupInterval <= 0 {
		c.CleanupInterval = time.Minute
	}
	if c.IdempotentResultTTL <= 0 {
		c.IdempotentResultTTL = 5 * time.Minute
	}
	if c.StreamRetention <= 0 {
		c.StreamRetention = 24 * time.Hour
	}
	if c.CleanupBatchSize <= 0 {
		c.CleanupBatchSize = 1000
	}
	if c.CleanupChunkPause <= 0 {
		c.CleanupChunkPause = 100 * time.Millisecond
	}

	c.TablePrefix = strings.TrimRight(c.TablePrefix, "_")
	if c.TablePrefix == "" {
		c.TablePrefix = "cf"
	}

	if c.Outbox.PollInterval <= 0 {
		c.Outbox.PollInterval = 50 * time.Millisecond
	}
	if c.Outbox.BatchSize <= 0 {
		c.Outbox.BatchSize = 1000
	}
	if c.Outbox.AdvisoryLockBaseID == 0 {
		c.Outbox.AdvisoryLockBaseID = 726966531
	}
	if c.Outbox.AdvisoryLockRetryInterval <= 0 {
		c.Outbox.AdvisoryLockRetryInterval = 5 * time.Second
	}
	if c.ReplicaPoolSize <= 0 {
		c.ReplicaPoolSize = c.PoolSize
	}
	if c.PartitionLookaheadDays <= 0 {
		c.PartitionLookaheadDays = 2
	}
	// PartitionRetentionDays is intentionally NOT defaulted here. The
	// configtypes layer (centrifugo config tag default:"7") handles the
	// production default. Direct Go-level construction must set this
	// explicitly: positive = days of retention, 0 = unlimited (the
	// pgoutbox.Partitioner guard treats 0 as a no-op DROP). Defaulting
	// to 7 here would make it impossible to express "unlimited retention"
	// without a sentinel like -1.
}

// PostgresStreamBroker is a centrifuge.Broker implementation backed by PostgreSQL.
type PostgresStreamBroker struct {
	node         *centrifuge.Node
	conf         PostgresStreamBrokerConfig
	names        pgNames
	pool         *pgxpool.Pool
	readPools    []*pgxpool.Pool
	notifyPool   *pgxpool.Pool // Dedicated single-conn pool for LISTEN; nil = use pool
	eventHandler centrifuge.BrokerEventHandler
	closeCh      chan struct{}
	closeOnce    sync.Once
	running      atomic.Bool
	cancelCtx    context.Context
	cancelFunc   context.CancelFunc
	notifyCh     chan struct{}
	sampler      *metricsSampler
}

var _ centrifuge.Broker = (*PostgresStreamBroker)(nil)

// NewPostgresStreamBroker constructs a new PostgresStreamBroker. Call EnsureSchema
// after construction and then RegisterBrokerEventHandler.
func NewPostgresStreamBroker(n *centrifuge.Node, conf PostgresStreamBrokerConfig) (*PostgresStreamBroker, error) {
	conf.setDefaults()

	if conf.DSN == "" {
		return nil, errors.New("postgres stream broker: DSN is required")
	}

	ctx := context.Background()

	poolConfig, err := pgxpool.ParseConfig(conf.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres stream broker: parse config: %w", err)
	}
	poolConfig.MaxConns = int32(conf.PoolSize)
	if conf.TLS.Enabled {
		tlsCfg, err := conf.TLS.ToGoTLSConfig("postgres-stream-broker")
		if err != nil {
			return nil, fmt.Errorf("postgres stream broker: TLS config: %w", err)
		}
		poolConfig.ConnConfig.TLSConfig = tlsCfg
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("postgres stream broker: create pool: %w", err)
	}

	var readPools []*pgxpool.Pool
	for i, dsn := range conf.ReplicaDSN {
		rcfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			pool.Close()
			for _, rp := range readPools {
				rp.Close()
			}
			return nil, fmt.Errorf("postgres stream broker: parse replica %d config: %w", i, err)
		}
		rcfg.MaxConns = int32(conf.ReplicaPoolSize)
		if conf.TLS.Enabled {
			tlsCfg, err := conf.TLS.ToGoTLSConfig("postgres-stream-broker")
			if err != nil {
				pool.Close()
				for _, rp := range readPools {
					rp.Close()
				}
				return nil, fmt.Errorf("postgres stream broker: replica TLS config: %w", err)
			}
			rcfg.ConnConfig.TLSConfig = tlsCfg
		}
		rp, err := pgxpool.NewWithConfig(ctx, rcfg)
		if err != nil {
			pool.Close()
			for _, p := range readPools {
				p.Close()
			}
			return nil, fmt.Errorf("postgres stream broker: create replica %d pool: %w", i, err)
		}
		readPools = append(readPools, rp)
	}

	cancelCtx, cancelFunc := context.WithCancel(context.Background())

	b := &PostgresStreamBroker{
		node:       n,
		conf:       conf,
		names:      newPgNames(conf.TablePrefix, conf.BinaryData),
		pool:       pool,
		readPools:  readPools,
		closeCh:    make(chan struct{}),
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
	}
	b.sampler = newMetricsSampler(b)
	if conf.UseNotify {
		b.notifyCh = make(chan struct{}, 1)
		if conf.NotifyDSN != "" {
			nCfg, err := pgxpool.ParseConfig(conf.NotifyDSN)
			if err != nil {
				pool.Close()
				for _, rp := range readPools {
					rp.Close()
				}
				cancelFunc()
				return nil, fmt.Errorf("postgres stream broker: parse notify DSN: %w", err)
			}
			nCfg.MaxConns = 1
			if conf.TLS.Enabled {
				tlsCfg, err := conf.TLS.ToGoTLSConfig("postgres-stream-broker")
				if err != nil {
					pool.Close()
					for _, rp := range readPools {
						rp.Close()
					}
					cancelFunc()
					return nil, fmt.Errorf("postgres stream broker: notify TLS config: %w", err)
				}
				nCfg.ConnConfig.TLSConfig = tlsCfg
			}
			nPool, err := pgxpool.NewWithConfig(ctx, nCfg)
			if err != nil {
				pool.Close()
				for _, rp := range readPools {
					rp.Close()
				}
				cancelFunc()
				return nil, fmt.Errorf("postgres stream broker: connect notify: %w", err)
			}
			b.notifyPool = nPool
		}
	}
	return b, nil
}

// ReliableDelivery reports whether this broker guarantees no-gaps delivery to
// local subscribers, so the centrifuge node can skip periodic position sync
// requests. True when polling the PG outbox directly (shard lock guarantees
// commit order = id order, cursor-based polling reads every row). False when
// broker fan-out is enabled — the Redis/Nats fan-out leg can drop messages.
func (e *PostgresStreamBroker) ReliableDelivery() bool {
	return e.conf.Broker == nil
}

// Close shuts down the broker.
func (e *PostgresStreamBroker) Close(ctx context.Context) error {
	e.closeOnce.Do(func() {
		e.cancelFunc()
		close(e.closeCh)
		if e.conf.Broker != nil {
			if c, ok := e.conf.Broker.(centrifuge.Closer); ok {
				_ = c.Close(ctx)
			}
		}
		for _, rp := range e.readPools {
			rp.Close()
		}
		if e.notifyPool != nil {
			e.notifyPool.Close()
		}
		e.pool.Close()
	})
	return nil
}

// RegisterBrokerEventHandler registers the event handler and starts background workers.
func (e *PostgresStreamBroker) RegisterBrokerEventHandler(h centrifuge.BrokerEventHandler) error {
	e.eventHandler = h

	if e.running.Swap(true) {
		return errors.New("postgres stream broker: already running")
	}

	// Pre-initialize the outbox cursor before launching worker goroutines so
	// that any message published after RegisterBrokerEventHandler returns is
	// guaranteed to be delivered. Without this, a goroutine scheduled late
	// could call initOutboxCursor after a message was already inserted, see
	// that ID as MAX(id), and silently skip it.
	initialCursor, err := e.initOutboxCursor(e.cancelCtx, e.pool)
	if err != nil {
		e.logErrorMsg("pre-init outbox cursor", err)
		initialCursor = 0
	}

	if e.conf.Broker != nil {
		if err := e.conf.Broker.RegisterBrokerEventHandler(h); err != nil {
			return fmt.Errorf("postgres stream broker: register inner broker: %w", err)
		}
		for i := 0; i < e.conf.NumShards; i++ {
			go e.runOutboxWorkerWithLock(i, initialCursor)
		}
	} else {
		if e.conf.UseNotify {
			go e.runNotificationListener()
		}
		for i := 0; i < e.conf.NumShards; i++ {
			go e.runOutboxWorker(i, initialCursor)
		}
	}

	go e.runCleanupWorker()
	go e.runPartitionWorker() // unconditional — schema is always partitioned
	return nil
}

// Subscribe is a no-op when running without an inner broker (every node polls
// the outbox). With fanout enabled, delegates to the inner Broker.
func (e *PostgresStreamBroker) Subscribe(channels ...string) error {
	if e.conf.Broker != nil {
		return e.conf.Broker.Subscribe(channels...)
	}
	return nil
}

// Unsubscribe mirrors Subscribe.
func (e *PostgresStreamBroker) Unsubscribe(channels ...string) error {
	if e.conf.Broker != nil {
		return e.conf.Broker.Unsubscribe(channels...)
	}
	return nil
}

// getReadPool returns the appropriate pool for read queries. If allowCached
// is true and replicas are configured, routes by shard hash to a replica.
func (e *PostgresStreamBroker) getReadPool(channel string, allowCached bool) *pgxpool.Pool {
	if !allowCached || len(e.readPools) == 0 {
		return e.pool
	}
	shardID := abs32(hashtext(channel)) % e.conf.NumShards
	replicaIdx := shardID % len(e.readPools)
	return e.readPools[replicaIdx]
}

// dataParam wraps a []byte for use as a SQL parameter. With JSONB columns,
// wrap as json.RawMessage so pgx encodes it as JSON text. With BYTEA, pass
// as plain []byte.
func (e *PostgresStreamBroker) dataParam(b []byte) any {
	if b == nil {
		return nil
	}
	if !e.conf.BinaryData {
		return json.RawMessage(b)
	}
	return b
}

// rawDataBytes reads a data column (JSONB or BYTEA depending on BinaryData)
// using the correct format-aware parser.
func (e *PostgresStreamBroker) rawDataBytes(a *byteArena, b []byte, format int16) []byte {
	if e.conf.BinaryData {
		return pgRawBytes(a, b, format)
	}
	return pgRawJSONBBytes(a, b, format)
}

func (e *PostgresStreamBroker) dataType() string {
	if e.conf.BinaryData {
		return "BYTEA"
	}
	return "JSONB"
}

func (e *PostgresStreamBroker) logErrorMsg(msg string, err error) {
	log.Error().Err(err).Str("broker", e.conf.Name).Msg("postgres stream broker: " + msg)
}

// pgColFormatsFromRows extracts per-column wire format codes from pgx rows.
func pgColFormatsFromRows(rows pgx.Rows) pgColFormats {
	descs := rows.FieldDescriptions()
	fmts := make(pgColFormats, len(descs))
	for i, d := range descs {
		fmts[i] = d.Format
	}
	return fmts
}

// execSchemaWithRetry executes idempotent schema SQL, retrying on transient
// conflicts: deadlock (40P01) and "tuple concurrently updated" (XX000).
func (e *PostgresStreamBroker) execSchemaWithRetry(ctx context.Context, sql string) error {
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		_, err := e.pool.Exec(ctx, sql)
		if err == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "XX000") && attempt < maxRetries-1 {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		return &SchemaError{
			Object: SchemaObject{Type: "schema", Name: ""},
			Op:     "create",
			Err:    err,
		}
	}
	return nil
}
