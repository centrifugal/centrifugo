package pgmapbroker

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

	"github.com/centrifugal/centrifuge"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed internal/sql/schema_jsonb.sql
var postgresSchemaJSONBSQL string

//go:embed internal/sql/schema_binary.sql
var postgresSchemaBinarySQL string

// schemaVersion is the current schema version. Bump when adding migrations.
var schemaVersion = 1

// schemaMigrations maps target version to migration SQL.
// Each migration must handle BOTH prefixes and be idempotent.
// Version 1 is the baseline (applied via full DDL). Migrations start at 2.
var schemaMigrations = map[int]string{}

// execSchemaWithRetry executes idempotent schema SQL, retrying on transient
// conflicts: deadlock (40P01) and "tuple concurrently updated" (XX000).
// The latter occurs when concurrent CREATE OR REPLACE FUNCTION statements
// race on the same function (e.g. during rolling deploys).
func (e *PostgresMapBroker) execSchemaWithRetry(ctx context.Context, sql string) error {
	const maxRetries = 3
	for attempt := range maxRetries {
		_, err := e.pool.Exec(ctx, sql)
		if err == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "XX000") && attempt < maxRetries-1 {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if errors.As(err, &pgErr) {
			return &SchemaError{
				Object: SchemaObject{Type: "schema", Name: pgErr.TableName},
				Op:     "create",
				Err:    err,
			}
		}
		return &SchemaError{
			Object: SchemaObject{Type: "schema", Name: ""},
			Op:     "create",
			Err:    err,
		}
	}
	return nil
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

// pgNames holds precomputed table/function names based on BinaryData mode.
// When BinaryData=false (default): prefix = "cf_map_"
// When BinaryData=true:            prefix = "cf_binary_map_"
type pgNames struct {
	stream, state, meta, idempotency, shardLock, schemaVersion string // table names
	publish, remove, expireKeys                                string // function names
	notifyChannel                                              string // pg_notify channel name
}

func newPgNames(binary bool) pgNames {
	p := "cf_map_"
	if binary {
		p = "cf_binary_map_"
	}
	return pgNames{
		stream:        p + "stream",
		state:         p + "state",
		meta:          p + "meta",
		idempotency:   p + "idempotency",
		shardLock:     p + "shard_lock",
		schemaVersion: p + "schema_version",
		publish:       p + "publish",
		remove:        p + "remove",
		expireKeys:    p + "expire_keys",
		notifyChannel: p + "stream_notify",
	}
}

// PostgresMapBroker is MapBroker implementation using PostgreSQL for persistent
// map subscriptions. It provides durability, CAS operations, and transactional
// publishing from SQL.
//
// Key features:
//   - Dual ID system: global `id` for polling, per-channel `offset` for Centrifuge
//   - All nodes independently poll the stream table (SQL SELECT is reliable)
//   - Optional LISTEN/NOTIFY for low-latency outbox wakeup
//   - Full ACID transactions for atomic CAS operations
//   - Optional read replica support for scaling reads
//
// Use cases: collaborative boards, document editing, inventory/booking systems,
// game lobbies with persistent state.

// durationToIntervalString converts a time.Duration to a PostgreSQL interval string.
// Uses milliseconds for precision.
func durationToIntervalString(d time.Duration) string {
	ms := d.Milliseconds()
	if ms > 0 {
		return strconv.FormatInt(ms, 10) + " milliseconds"
	}
	// Sub-millisecond duration — round up to 1 millisecond minimum.
	return "1 milliseconds"
}

type PostgresMapBroker struct {
	node         *centrifuge.Node
	conf         PostgresMapBrokerConfig
	names        pgNames
	pool         *pgxpool.Pool   // Primary pool for writes
	readPools    []*pgxpool.Pool // One per replica; empty = use primary
	eventHandler centrifuge.BrokerEventHandler
	closeCh      chan struct{}
	closeOnce    sync.Once
	running      atomic.Bool
	cancelCtx    context.Context
	cancelFunc   context.CancelFunc
	notifyCh     chan struct{} // nil when UseNotify is false
}

var _ centrifuge.MapBroker = (*PostgresMapBroker)(nil)

// OutboxConfig configures the outbox-based delivery mode.
// Every node independently polls cf_map_stream — no advisory locks needed.
type OutboxConfig struct {
	// PollInterval is how often to poll for new stream entries when idle.
	// Default: 50ms
	PollInterval time.Duration

	// BatchSize is the maximum number of rows to process per batch.
	// Default: 1000
	BatchSize int

	// AdvisoryLockBaseID is the base ID for PostgreSQL advisory locks used to
	// claim shards when Broker fan-out is enabled. Lock ID = AdvisoryLockBaseID + shardID.
	// Default: 726966530.
	AdvisoryLockBaseID int64

	// AdvisoryLockRetryInterval is how often to retry advisory lock acquisition
	// when Broker fan-out is enabled. Default: 5s.
	AdvisoryLockRetryInterval time.Duration
}

// PostgresMapBrokerConfig configures the PostgreSQL map broker.
type PostgresMapBrokerConfig struct {
	// Name of broker, for observability purposes – i.e. becomes part of metrics/logs labels.
	// By default, empty string is used.
	Name string

	// DSN is the primary PostgreSQL connection string for writes.
	// Example: "postgres://user:pass@localhost:5432/dbname?sslmode=disable"
	DSN string

	// PoolSize sets the maximum number of connections in the pool.
	// Default: 32
	PoolSize int

	// NumShards is the total number of shards for parallel delivery workers.
	// Channels are distributed across shards using hash(channel) % NumShards.
	// Default: 16
	NumShards int

	// TTLCheckInterval is how often to check for expired keys.
	// Default: 1s
	TTLCheckInterval time.Duration

	// CleanupInterval is how often to clean up expired stream/meta/idempotency entries.
	// Default: 1m
	CleanupInterval time.Duration

	// IdempotentResultTTL is the default TTL for idempotency keys.
	// Default: 5m
	IdempotentResultTTL time.Duration

	// Outbox configures the outbox-based delivery mode.
	Outbox OutboxConfig

	// BinaryData uses BYTEA columns instead of JSONB for data fields.
	// Default: false (JSONB — suitable for JSON payloads, enables JSONB queries).
	// Set to true if data payloads are not valid JSON (binary/protobuf).
	BinaryData bool

	// StreamRetention controls how long stream entries are kept.
	// Cleanup worker deletes entries older than this. Default: 24h.
	StreamRetention time.Duration

	// UseNotify enables LISTEN/NOTIFY for low-latency outbox wakeup.
	// When false (default), outbox worker uses PollInterval-based polling only.
	// When true, a listener goroutine wakes the worker immediately on new entries.
	UseNotify bool

	// SkipShardLock disables per-shard serialization of stream inserts.
	// Default false: shard locking ensures outbox workers never miss rows.
	// Set to true when using WAL-based delivery (future).
	SkipShardLock bool

	// ReplicaDSN is an optional list of read replica connection strings.
	// When set, ReadState queries with AllowCached=true are distributed
	// across replicas using shard-based routing for consistency:
	//   hash(channel) % NumShards → shard_id % len(replicas) → replica
	// Default: empty (all reads go to primary).
	ReplicaDSN []string

	// ReplicaPoolSize sets max connections per replica pool.
	// Default: same as PoolSize.
	ReplicaPoolSize int

	// Broker is an optional Broker (e.g. RedisBroker) for PUB/SUB fan-out.
	// When set, outbox workers use advisory locks (one worker per shard across
	// all nodes) and publish via Broker instead of HandlePublication.
	// Subscribe/Unsubscribe are delegated to this Broker.
	// When nil, every node polls independently (current behavior).
	Broker centrifuge.Broker

	// Partitioning enables automatic daily partitioning of the stream table.
	// This is purely an optimization — without it, the broker works correctly
	// using simple DELETE-based cleanup. Partitioning helps at scale where
	// DROP TABLE (instant) is better than DELETE + VACUUM overhead.
	// Default: false.
	Partitioning bool

	// PartitionRetentionDays: how many days of partitions to keep.
	// Only used when Partitioning is true. Default: 3.
	PartitionRetentionDays int

	// PartitionLookaheadDays: how many future partitions to pre-create.
	// Only used when Partitioning is true. Default: 2.
	PartitionLookaheadDays int
}

func (c *PostgresMapBrokerConfig) setDefaults() {
	if c.PoolSize <= 0 {
		c.PoolSize = 32
	}
	if c.NumShards <= 0 {
		c.NumShards = 16
	}
	if c.TTLCheckInterval <= 0 {
		c.TTLCheckInterval = time.Second
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

	// Outbox config defaults
	if c.Outbox.PollInterval <= 0 {
		c.Outbox.PollInterval = 50 * time.Millisecond
	}
	if c.Outbox.BatchSize <= 0 {
		c.Outbox.BatchSize = 1000
	}
	if c.Outbox.AdvisoryLockBaseID == 0 {
		c.Outbox.AdvisoryLockBaseID = 726966530
	}
	if c.Outbox.AdvisoryLockRetryInterval <= 0 {
		c.Outbox.AdvisoryLockRetryInterval = 5 * time.Second
	}

	if c.ReplicaPoolSize <= 0 {
		c.ReplicaPoolSize = c.PoolSize
	}
	if c.PartitionRetentionDays <= 0 {
		c.PartitionRetentionDays = 3
	}
	if c.PartitionLookaheadDays <= 0 {
		c.PartitionLookaheadDays = 2
	}
}

// NewPostgresMapBroker creates a new PostgreSQL map broker.
func NewPostgresMapBroker(n *centrifuge.Node, conf PostgresMapBrokerConfig) (*PostgresMapBroker, error) {
	conf.setDefaults()

	if conf.DSN == "" {
		return nil, errors.New("postgres map broker: DSN is required")
	}

	ctx := context.Background()

	// Configure primary pool
	poolConfig, err := pgxpool.ParseConfig(conf.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres map broker: parse config: %w", err)
	}
	poolConfig.MaxConns = int32(conf.PoolSize)

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("postgres map broker: connect primary: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres map broker: ping primary: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	e := &PostgresMapBroker{
		node:       n,
		conf:       conf,
		names:      newPgNames(conf.BinaryData),
		pool:       pool,
		closeCh:    make(chan struct{}),
		cancelCtx:  ctx,
		cancelFunc: cancel,
	}

	if conf.UseNotify {
		e.notifyCh = make(chan struct{}, 1)
	}

	// Create replica pools if configured
	if len(conf.ReplicaDSN) > 0 {
		for _, connStr := range conf.ReplicaDSN {
			replicaConfig, err := pgxpool.ParseConfig(connStr)
			if err != nil {
				pool.Close()
				for _, rp := range e.readPools {
					rp.Close()
				}
				cancel()
				return nil, fmt.Errorf("postgres map broker: parse replica config: %w", err)
			}
			replicaConfig.MaxConns = int32(conf.ReplicaPoolSize)

			rp, err := pgxpool.NewWithConfig(context.Background(), replicaConfig)
			if err != nil {
				pool.Close()
				for _, rp := range e.readPools {
					rp.Close()
				}
				cancel()
				return nil, fmt.Errorf("postgres map broker: connect replica: %w", err)
			}
			e.readPools = append(e.readPools, rp)
		}
	}

	return e, nil
}

// getReadPool returns the pool for reading the given channel.
// Only routes to replica when allowCached is true AND replicas are configured.
// Routes by shard: hash(channel) % NumShards % len(readPools) → replica index.
func (e *PostgresMapBroker) getReadPool(channel string, allowCached bool) *pgxpool.Pool {
	if !allowCached || len(e.readPools) == 0 {
		return e.pool
	}
	shardID := abs32(hashtext(channel)) % e.conf.NumShards
	replicaIdx := shardID % len(e.readPools)
	return e.readPools[replicaIdx]
}

// hashtext approximates PostgreSQL hashtext() for shard routing.
func hashtext(s string) int32 {
	// Use FNV-like hash matching PostgreSQL hashtext behavior.
	// We only need consistency within a process, not cross-process compatibility with PG,
	// since shard routing is for outbox/replica selection, not for correctness.
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

// RegisterEventHandler registers the event handler and starts background workers.
func (e *PostgresMapBroker) RegisterEventHandler(h centrifuge.BrokerEventHandler) error {
	e.eventHandler = h

	if e.running.Swap(true) {
		return errors.New("postgres map broker: already running")
	}

	if e.conf.Broker != nil {
		// When inner Broker is configured, register it for PUB/SUB fan-out
		// and use advisory lock workers to ensure only one node per shard polls.
		if err := e.conf.Broker.RegisterBrokerEventHandler(h); err != nil {
			return fmt.Errorf("postgres map broker: register inner broker: %w", err)
		}
		for i := 0; i < e.conf.NumShards; i++ {
			go e.runOutboxWorkerWithLock(i)
		}
	} else {
		if e.conf.UseNotify {
			go e.runNotificationListener()
		}
		// Start outbox workers: one per shard. Per-shard serialization (FOR UPDATE
		// on shard_lock) + one-shard-per-worker eliminates BIGSERIAL gaps.
		for i := 0; i < e.conf.NumShards; i++ {
			go e.runOutboxWorker(i)
		}
	}

	go e.runTTLExpirationWorker()
	go e.runCleanupLagWorker()
	go e.runCleanupWorker()
	if e.conf.Partitioning {
		go e.runPartitionWorker()
	}

	return nil
}

// Close shuts down the broker.
func (e *PostgresMapBroker) Close(ctx context.Context) error {
	e.closeOnce.Do(func() {
		e.cancelFunc() // Cancel context to unblock WaitForNotification
		close(e.closeCh)
		if e.conf.Broker != nil {
			if closer, ok := e.conf.Broker.(centrifuge.Closer); ok {
				_ = closer.Close(ctx)
			}
		}
		for _, rp := range e.readPools {
			rp.Close()
		}
		e.pool.Close()
	})
	return nil
}

// SchemaObject identifies a database object involved in a schema error.
type SchemaObject struct {
	Type string // "table", "index", "function"
	Name string
}

// SchemaError wraps a schema-related error with object and operation info.
type SchemaError struct {
	Object SchemaObject
	Op     string // "create", "verify"
	Err    error
}

func (e *SchemaError) Error() string {
	return fmt.Sprintf("schema %s %s %q: %v", e.Op, e.Object.Type, e.Object.Name, e.Err)
}

func (e *SchemaError) Unwrap() error {
	return e.Err
}

// EnsureSchema creates all required database objects idempotently.
// It creates BOTH JSONB and BYTEA schema variants in a single call,
// regardless of the BinaryData config (which only controls which variant
// is used at runtime). This ensures both schemas are always available.
//
// Schema versioning uses an integer version stored in cf_map_schema_version.
// On startup, if the version matches and a probe query succeeds, all DDL
// is skipped (fast path). Otherwise, full DDL is re-applied (idempotent)
// and any pending migrations are executed in order.
//
// This method is safe to call concurrently from multiple nodes — all DDL
// uses CREATE IF NOT EXISTS / CREATE OR REPLACE, and all migrations must
// be idempotent (e.g. ADD COLUMN IF NOT EXISTS).
func (e *PostgresMapBroker) EnsureSchema(ctx context.Context) error {
	// 1. Read version: fast path if version matches and probe succeeds.
	var dbVersion int
	err := e.pool.QueryRow(ctx,
		`SELECT schema_version FROM cf_map_schema_version WHERE id = 1`,
	).Scan(&dbVersion)
	if err == nil && dbVersion == schemaVersion {
		// Verify a critical table actually exists — guards against partial
		// schema loss (e.g. schema_version survived but tables were dropped).
		if _, probeErr := e.pool.Exec(ctx, fmt.Sprintf(
			`SELECT 1 FROM %s LIMIT 0`, e.names.stream)); probeErr == nil {
			return nil
		}
	}
	if err != nil {
		dbVersion = 0 // Table doesn't exist or other error — treat as fresh.
	}

	// 2. Run DDL for both variants (separate transactions for lock isolation).
	//    Split into DDL (tables+indexes) and functions, executed as separate
	//    transactions to reduce lock scope. Each part retries on deadlock/conflict.
	jsonbDDL, jsonbFuncs := splitSchemaSQL(postgresSchemaJSONBSQL)
	binaryDDL, binaryFuncs := splitSchemaSQL(postgresSchemaBinarySQL)
	for _, sql := range []string{jsonbDDL, binaryDDL, jsonbFuncs, binaryFuncs} {
		if sql == "" {
			continue
		}
		if err := e.execSchemaWithRetry(ctx, sql); err != nil {
			return err
		}
	}

	// 3. Populate/trim shard_lock for BOTH prefixes.
	for _, prefix := range []string{"cf_map_", "cf_binary_map_"} {
		shardLock := prefix + "shard_lock"
		if _, err := e.pool.Exec(ctx, fmt.Sprintf(
			`INSERT INTO %s (shard_id) SELECT generate_series(0, $1 - 1) ON CONFLICT DO NOTHING`,
			shardLock), e.conf.NumShards); err != nil {
			return &SchemaError{
				Object: SchemaObject{Type: "table", Name: shardLock},
				Op:     "create",
				Err:    fmt.Errorf("populate shard_lock: %w", err),
			}
		}
		if _, err := e.pool.Exec(ctx, fmt.Sprintf(
			`DELETE FROM %s WHERE shard_id >= $1`,
			shardLock), e.conf.NumShards); err != nil {
			return &SchemaError{
				Object: SchemaObject{Type: "table", Name: shardLock},
				Op:     "create",
				Err:    fmt.Errorf("trim shard_lock: %w", err),
			}
		}
	}

	// 4. Partitioning (if configured, uses e.names for the active prefix).
	if e.conf.Partitioning {
		if err := e.ensurePartitionedStream(ctx); err != nil {
			return err
		}
	}

	// 5. Migration loop (SKIP if dbVersion == 0, fresh install — DDL has latest).
	if dbVersion > 0 {
		for v := dbVersion + 1; v <= schemaVersion; v++ {
			if sql, ok := schemaMigrations[v]; ok {
				if err := e.execSchemaWithRetry(ctx, sql); err != nil {
					return err
				}
			}
		}
	}

	// 6. Update version (always, ensures both tables are current).
	for _, prefix := range []string{"cf_map_", "cf_binary_map_"} {
		if _, err := e.pool.Exec(ctx, fmt.Sprintf(
			`UPDATE %sschema_version SET schema_version = $1 WHERE id = 1`,
			prefix), schemaVersion); err != nil {
			// Non-fatal: schema was created successfully, just version tracking failed.
			e.logErrorMsg("schema version update", err)
		}
	}

	return nil
}

// Subscribe delegates to inner Broker when configured, otherwise no-op.
func (e *PostgresMapBroker) Subscribe(ch string) error {
	if e.conf.Broker != nil {
		return e.conf.Broker.Subscribe(ch)
	}
	return nil
}

// Unsubscribe delegates to inner Broker when configured, otherwise no-op.
func (e *PostgresMapBroker) Unsubscribe(ch string) error {
	if e.conf.Broker != nil {
		return e.conf.Broker.Unsubscribe(ch)
	}
	return nil
}

// parseSuppressReason converts SQL suppress_reason string to SuppressReason type.
func parseSuppressReason(reason *string) centrifuge.SuppressReason {
	if reason == nil {
		return centrifuge.SuppressReasonNone
	}
	switch *reason {
	case "idempotency":
		return centrifuge.SuppressReasonIdempotency
	case "position_mismatch":
		return centrifuge.SuppressReasonPositionMismatch
	case "key_exists":
		return centrifuge.SuppressReasonKeyExists
	case "key_not_found":
		return centrifuge.SuppressReasonKeyNotFound
	case "version":
		return centrifuge.SuppressReasonVersion
	default:
		return centrifuge.SuppressReasonNone
	}
}

// Publish publishes data to a map channel using the cf_map_publish SQL function.
func (e *PostgresMapBroker) Publish(ctx context.Context, ch string, key string, opts centrifuge.MapPublishOptions) (centrifuge.MapUpdateResult, error) {
	// Resolve and validate channel options.
	chOpts, err := centrifuge.ResolveAndValidateMapChannelOptions(e.node.Config().GetMapChannelOptions, ch)
	if err != nil {
		return centrifuge.MapUpdateResult{}, err
	}

	// Reject CAS and Version in ephemeral mode.
	if chOpts.SyncMode == centrifuge.MapSyncEphemeral {
		if opts.ExpectedPosition != nil {
			return centrifuge.MapUpdateResult{}, errors.New("CAS (ExpectedPosition) requires SyncMode Converging")
		}
		if opts.Version > 0 {
			return centrifuge.MapUpdateResult{}, errors.New("version-based dedup requires SyncMode Converging")
		}
	}

	// Prepare client info fields
	var clientID, userID *string
	var connInfo, chanInfo []byte
	var publishedAt *time.Time
	if opts.ClientInfo != nil {
		if opts.ClientInfo.ClientID != "" {
			clientID = &opts.ClientInfo.ClientID
		}
		if opts.ClientInfo.UserID != "" {
			userID = &opts.ClientInfo.UserID
		}
		connInfo = opts.ClientInfo.ConnInfo
		chanInfo = opts.ClientInfo.ChanInfo
		now := time.Now()
		publishedAt = &now
	}

	// Prepare tags as json.RawMessage so pgx encodes it as JSON (not hex bytea)
	// in both extended and simple protocol modes.
	var tagsJSON json.RawMessage
	if opts.Tags != nil {
		tagsJSON, _ = json.Marshal(opts.Tags)
	}

	// Prepare key mode
	var keyMode *string
	if opts.KeyMode != centrifuge.KeyModeReplace {
		km := string(opts.KeyMode)
		keyMode = &km
	}

	// Prepare TTLs as interval strings.
	var keyTTL, metaTTL, idempotencyTTL *string
	if chOpts.KeyTTL > 0 {
		s := durationToIntervalString(chOpts.KeyTTL)
		keyTTL = &s
	}
	if chOpts.MetaTTL > 0 {
		s := durationToIntervalString(chOpts.MetaTTL)
		metaTTL = &s
	}
	idempotentResultTTL := opts.IdempotentResultTTL
	if idempotentResultTTL == 0 {
		idempotentResultTTL = e.conf.IdempotentResultTTL
	}
	if opts.IdempotencyKey != "" && idempotentResultTTL > 0 {
		s := durationToIntervalString(idempotentResultTTL)
		idempotencyTTL = &s
	}

	// Prepare expected offset
	var expectedOffset *int64
	if opts.ExpectedPosition != nil {
		eo := int64(opts.ExpectedPosition.Offset)
		expectedOffset = &eo
	}

	// Prepare score
	var score *int64
	ordered := chOpts.Ordered
	if ordered || opts.Score != 0 {
		score = &opts.Score
	}

	// Prepare per-key version (stored in state, used for per-key version check)
	var keyVersion *int64
	var keyVersionEpoch *string
	if opts.Version > 0 && key != "" {
		v := int64(opts.Version)
		keyVersion = &v
		if opts.VersionEpoch != "" {
			keyVersionEpoch = &opts.VersionEpoch
		}
	}

	// Prepare idempotency key
	var idempotencyKey *string
	if opts.IdempotencyKey != "" {
		idempotencyKey = &opts.IdempotencyKey
	}

	// Call cf_map_publish function
	numShards := e.conf.NumShards

	var id *int64
	var channelOffset int64
	var epoch string
	var suppressed bool
	var suppressReason *string
	var currentData []byte
	var currentOffset *int64

	useDelta := opts.UseDelta && len(opts.StreamData) == 0

	// StreamData is stored in stream; state always uses Data.
	var streamData []byte
	if len(opts.StreamData) > 0 {
		streamData = opts.StreamData
	}

	err = e.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT result_id, channel_offset, epoch, suppressed, suppress_reason, current_data, current_offset
		FROM %s($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::interval, $12::interval, $13, $14, $15, $16, $17, $18, $19, $20::interval, $21, $22, $23, $24, $25)
	`, e.names.publish),
		ch, key, e.dataParam(opts.Data), tagsJSON,
		clientID, userID, e.dataParam(connInfo), e.dataParam(chanInfo), publishedAt,
		keyMode, keyTTL, metaTTL,
		expectedOffset, score, nil, nil, // p_version, p_version_epoch (unused, per-key version used instead)
		keyVersion, keyVersionEpoch,
		idempotencyKey, idempotencyTTL, opts.RefreshTTLOnSuppress,
		useDelta, numShards, e.dataParam(streamData), e.conf.SkipShardLock,
	).Scan(&id, &channelOffset, &epoch, &suppressed, &suppressReason, &currentData, &currentOffset)

	if err != nil {
		return centrifuge.MapUpdateResult{}, err
	}

	newPos := centrifuge.StreamPosition{Offset: uint64(channelOffset), Epoch: epoch}

	if suppressed {
		result := centrifuge.MapUpdateResult{
			Position:       newPos,
			Suppressed:     true,
			SuppressReason: parseSuppressReason(suppressReason),
		}
		// For position_mismatch, include current publication data
		if suppressReason != nil && *suppressReason == "position_mismatch" && currentOffset != nil {
			result.CurrentPublication = &centrifuge.Publication{
				Offset: uint64(*currentOffset),
				Key:    key,
				Data:   currentData,
			}
		}
		return result, nil
	}

	return centrifuge.MapUpdateResult{Position: newPos}, nil
}

// Remove removes a key from keyed state using the cf_map_remove SQL function.
func (e *PostgresMapBroker) Remove(ctx context.Context, ch string, key string, opts centrifuge.MapRemoveOptions) (centrifuge.MapUpdateResult, error) {
	// Resolve and validate channel options.
	chOpts, err := centrifuge.ResolveAndValidateMapChannelOptions(e.node.Config().GetMapChannelOptions, ch)
	if err != nil {
		return centrifuge.MapUpdateResult{}, err
	}

	// Reject CAS in ephemeral mode.
	if chOpts.SyncMode == centrifuge.MapSyncEphemeral {
		if opts.ExpectedPosition != nil {
			return centrifuge.MapUpdateResult{}, errors.New("CAS (ExpectedPosition) requires SyncMode Converging")
		}
	}

	// Prepare TTLs as interval strings.
	var metaTTL, idempotencyTTL *string
	if chOpts.MetaTTL > 0 {
		s := durationToIntervalString(chOpts.MetaTTL)
		metaTTL = &s
	}
	idempotentResultTTL := opts.IdempotentResultTTL
	if idempotentResultTTL == 0 {
		idempotentResultTTL = e.conf.IdempotentResultTTL
	}
	if opts.IdempotencyKey != "" && idempotentResultTTL > 0 {
		s := durationToIntervalString(idempotentResultTTL)
		idempotencyTTL = &s
	}

	// Prepare idempotency key
	var idempotencyKey *string
	if opts.IdempotencyKey != "" {
		idempotencyKey = &opts.IdempotencyKey
	}

	// Prepare expected position for CAS
	var expectedOffset *int64
	if opts.ExpectedPosition != nil {
		eo := int64(opts.ExpectedPosition.Offset)
		expectedOffset = &eo
	}

	// Call cf_map_remove function
	numShards := e.conf.NumShards

	var id *int64
	var channelOffset int64
	var epoch string
	var suppressed bool
	var suppressReason *string
	var currentData []byte
	var currentOffset *int64

	// Client info is not available in remove options
	var clientID, userID *string
	err = e.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT result_id, channel_offset, epoch, suppressed, suppress_reason, current_data, current_offset
		FROM %s($1, $2, $3, $4, $5, $6::interval, $7::interval, $8, $9, $10)
	`, e.names.remove),
		ch, key, clientID, userID, idempotencyKey, idempotencyTTL, metaTTL,
		numShards, expectedOffset, e.conf.SkipShardLock,
	).Scan(&id, &channelOffset, &epoch, &suppressed, &suppressReason, &currentData, &currentOffset)

	if err != nil {
		return centrifuge.MapUpdateResult{}, err
	}

	newPos := centrifuge.StreamPosition{Offset: uint64(channelOffset), Epoch: epoch}

	if suppressed {
		result := centrifuge.MapUpdateResult{
			Position:       newPos,
			Suppressed:     true,
			SuppressReason: parseSuppressReason(suppressReason),
		}
		if suppressReason != nil && *suppressReason == "position_mismatch" && currentOffset != nil {
			result.CurrentPublication = &centrifuge.Publication{
				Offset: uint64(*currentOffset),
				Key:    key,
				Data:   currentData,
			}
		}
		return result, nil
	}

	return centrifuge.MapUpdateResult{Position: newPos}, nil
}

// ReadState retrieves keyed state with revisions.
func (e *PostgresMapBroker) ReadState(ctx context.Context, ch string, opts centrifuge.MapReadStateOptions) (centrifuge.MapStateResult, error) {
	pool := e.getReadPool(ch, opts.AllowCached)

	// Limit=0 with no key: return just stream position, no transaction needed.
	if opts.Limit == 0 && opts.Key == "" {
		return e.readStatePosition(ctx, pool, ch, opts)
	}

	// Single key lookup (CAS read): batch meta + key query.
	if opts.Key != "" {
		return e.readStateKey(ctx, pool, ch, opts)
	}

	// Full/paginated state read.
	// Resolve channel options before building query (pure Go, no DB call).
	chOpts, err := centrifuge.ResolveAndValidateMapChannelOptions(e.node.Config().GetMapChannelOptions, ch)
	if err != nil {
		return centrifuge.MapStateResult{}, err
	}

	limit := opts.Limit
	if limit < 0 {
		limit = 100000
	}

	// Build state query based on ordering and cursor.
	stateTable := e.names.state
	ordered := chOpts.Ordered
	asc := opts.Asc
	var stateQuery string
	var stateArgs []any
	if ordered {
		if opts.Cursor == "" {
			if asc {
				stateQuery = fmt.Sprintf(`
					SELECT key, data, tags, key_offset, score, client_id, user_id, conn_info, chan_info
					FROM %s
					WHERE channel = $1
					ORDER BY score ASC, key ASC
					LIMIT $2
				`, stateTable)
			} else {
				stateQuery = fmt.Sprintf(`
					SELECT key, data, tags, key_offset, score, client_id, user_id, conn_info, chan_info
					FROM %s
					WHERE channel = $1
					ORDER BY score DESC, key DESC
					LIMIT $2
				`, stateTable)
			}
			stateArgs = []any{ch, limit + 1}
		} else {
			cursorScore, cursorKey := centrifuge.ParseOrderedCursor(opts.Cursor)
			cursorScoreInt, _ := strconv.ParseInt(cursorScore, 10, 64)
			if asc {
				stateQuery = fmt.Sprintf(`
					SELECT key, data, tags, key_offset, score, client_id, user_id, conn_info, chan_info
					FROM %s
					WHERE channel = $1
					  AND (score > $3 OR (score = $3 AND key > $4))
					ORDER BY score ASC, key ASC
					LIMIT $2
				`, stateTable)
			} else {
				stateQuery = fmt.Sprintf(`
					SELECT key, data, tags, key_offset, score, client_id, user_id, conn_info, chan_info
					FROM %s
					WHERE channel = $1
					  AND (score < $3 OR (score = $3 AND key < $4))
					ORDER BY score DESC, key DESC
					LIMIT $2
				`, stateTable)
			}
			stateArgs = []any{ch, limit + 1, cursorScoreInt, cursorKey}
		}
	} else {
		if opts.Cursor == "" {
			stateQuery = fmt.Sprintf(`
				SELECT key, data, tags, key_offset, score, client_id, user_id, conn_info, chan_info
				FROM %s
				WHERE channel = $1
				ORDER BY key
				LIMIT $2
			`, stateTable)
			stateArgs = []any{ch, limit + 1}
		} else {
			stateQuery = fmt.Sprintf(`
				SELECT key, data, tags, key_offset, score, client_id, user_id, conn_info, chan_info
				FROM %s
				WHERE channel = $1 AND key > $3
				ORDER BY key
				LIMIT $2
			`, stateTable)
			stateArgs = []any{ch, limit + 1, opts.Cursor}
		}
	}

	// Pipelined batch: meta + state in a single round trip with REPEATABLE READ.
	metaQuery := fmt.Sprintf(`SELECT top_offset, epoch FROM %s WHERE channel = $1`, e.names.meta)
	batch := &pgx.Batch{}
	batch.Queue("BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY")
	batch.Queue(metaQuery, ch)
	batch.Queue(stateQuery, stateArgs...)
	batch.Queue("COMMIT")

	br := pool.SendBatch(ctx, batch)

	// Consume BEGIN.
	if _, err := br.Exec(); err != nil {
		_ = br.Close()
		return centrifuge.MapStateResult{}, err
	}

	// Read meta.
	var topOffset int64
	var epoch string
	err = br.QueryRow().Scan(&topOffset, &epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = br.Close()
		if opts.Revision != nil && opts.Revision.Epoch != "" {
			return centrifuge.MapStateResult{}, centrifuge.ErrorUnrecoverablePosition
		}
		return centrifuge.MapStateResult{}, nil
	}
	if err != nil {
		_ = br.Close()
		return centrifuge.MapStateResult{}, err
	}

	streamPos := centrifuge.StreamPosition{Offset: uint64(topOffset), Epoch: epoch}

	if opts.Revision != nil && opts.Revision.Epoch != "" && opts.Revision.Epoch != epoch {
		_ = br.Close()
		return centrifuge.MapStateResult{Position: streamPos}, centrifuge.ErrorUnrecoverablePosition
	}

	// Read state rows.
	rows, err := br.Query()
	if err != nil {
		_ = br.Close()
		return centrifuge.MapStateResult{}, err
	}

	allocHint := limit + 1 // +1 for next-page detection row.
	if allocHint > 1001 {
		allocHint = 1001
	}
	arena := byteArena{buf: make([]byte, 0, allocHint*64)}
	backing := make([]centrifuge.Publication, 0, allocHint)
	pubs := make([]*centrifuge.Publication, 0, allocHint)
	// Use RawValues + arena to avoid per-row allocations.
	// Column order: key(0), data(1), tags(2), key_offset(3), score(4),
	//               client_id(5), user_id(6), conn_info(7), chan_info(8).
	var fmts pgColFormats
	for rows.Next() {
		if fmts == nil {
			fmts = pgColFormatsFromRows(rows)
		}
		raw := rows.RawValues()
		backing = append(backing, centrifuge.Publication{})
		p := &backing[len(backing)-1]
		p.Key = pgRawString(&arena, raw[0])
		p.Data = e.rawDataBytes(&arena, raw[1], fmts[1])
		p.Tags = pgRawJSONBMap(raw[2])
		p.Offset = pgRawUint64(raw[3], fmts[3])
		p.Score = pgRawInt64(raw[4], fmts[4])
		if raw[5] != nil {
			p.Info = &centrifuge.ClientInfo{
				ClientID: pgRawString(&arena, raw[5]),
				UserID:   pgRawString(&arena, raw[6]),
				ConnInfo: e.rawDataBytes(&arena, raw[7], fmts[7]),
				ChanInfo: e.rawDataBytes(&arena, raw[8], fmts[8]),
			}
		}
		pubs = append(pubs, p)
	}
	rows.Close()

	// Consume COMMIT.
	_, _ = br.Exec()
	_ = br.Close()

	if err := rows.Err(); err != nil {
		return centrifuge.MapStateResult{}, err
	}

	var nextCursor string
	if len(pubs) > limit {
		pubs = pubs[:limit]
		lastPub := pubs[limit-1]
		if ordered {
			nextCursor = centrifuge.MakeOrderedCursor(strconv.FormatInt(lastPub.Score, 10), lastPub.Key)
		} else {
			nextCursor = lastPub.Key
		}
	}

	return centrifuge.MapStateResult{Publications: pubs, Position: streamPos, Cursor: nextCursor}, nil
}

// readStatePosition returns just the stream position for a channel (no state entries).
// Used when Limit=0 with no key filter — a single meta query, no transaction needed.
func (e *PostgresMapBroker) readStatePosition(ctx context.Context, pool *pgxpool.Pool, ch string, opts centrifuge.MapReadStateOptions) (centrifuge.MapStateResult, error) {
	var topOffset int64
	var epoch string
	err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT top_offset, epoch FROM %s WHERE channel = $1`, e.names.meta), ch,
	).Scan(&topOffset, &epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		if opts.Revision != nil && opts.Revision.Epoch != "" {
			return centrifuge.MapStateResult{}, centrifuge.ErrorUnrecoverablePosition
		}
		return centrifuge.MapStateResult{}, nil
	}
	if err != nil {
		return centrifuge.MapStateResult{}, err
	}
	streamPos := centrifuge.StreamPosition{Offset: uint64(topOffset), Epoch: epoch}
	if opts.Revision != nil && opts.Revision.Epoch != "" && opts.Revision.Epoch != epoch {
		return centrifuge.MapStateResult{Position: streamPos}, centrifuge.ErrorUnrecoverablePosition
	}
	return centrifuge.MapStateResult{Position: streamPos}, nil
}

// readStateKey reads a single key from state (CAS read path).
// Uses pipelined batch: meta + key query in one round trip with REPEATABLE READ.
func (e *PostgresMapBroker) readStateKey(ctx context.Context, pool *pgxpool.Pool, ch string, opts centrifuge.MapReadStateOptions) (centrifuge.MapStateResult, error) {
	metaQuery := fmt.Sprintf(`SELECT top_offset, epoch FROM %s WHERE channel = $1`, e.names.meta)
	keyQuery := fmt.Sprintf(`
		SELECT key, data, tags, key_offset, client_id, user_id, conn_info, chan_info
		FROM %s
		WHERE channel = $1 AND key = $2
	`, e.names.state)

	batch := &pgx.Batch{}
	batch.Queue("BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY")
	batch.Queue(metaQuery, ch)
	batch.Queue(keyQuery, ch, opts.Key)
	batch.Queue("COMMIT")

	br := pool.SendBatch(ctx, batch)

	// Consume BEGIN.
	if _, err := br.Exec(); err != nil {
		_ = br.Close()
		return centrifuge.MapStateResult{}, err
	}

	// Read meta.
	var topOffset int64
	var epoch string
	err := br.QueryRow().Scan(&topOffset, &epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = br.Close()
		if opts.Revision != nil && opts.Revision.Epoch != "" {
			return centrifuge.MapStateResult{}, centrifuge.ErrorUnrecoverablePosition
		}
		return centrifuge.MapStateResult{}, nil
	}
	if err != nil {
		_ = br.Close()
		return centrifuge.MapStateResult{}, err
	}

	streamPos := centrifuge.StreamPosition{Offset: uint64(topOffset), Epoch: epoch}

	if opts.Revision != nil && opts.Revision.Epoch != "" && opts.Revision.Epoch != epoch {
		_ = br.Close()
		return centrifuge.MapStateResult{Position: streamPos}, centrifuge.ErrorUnrecoverablePosition
	}

	// Read key.
	var p centrifuge.Publication
	var tagsJSON []byte
	var clientID, userID *string
	var connInfo, chanInfo []byte
	err = br.QueryRow().Scan(&p.Key, &p.Data, &tagsJSON, &p.Offset, &clientID, &userID, &connInfo, &chanInfo)

	// Consume COMMIT and close batch before processing results.
	_, _ = br.Exec()
	_ = br.Close()

	if errors.Is(err, pgx.ErrNoRows) {
		return centrifuge.MapStateResult{Position: streamPos}, nil
	}
	if err != nil {
		return centrifuge.MapStateResult{}, err
	}
	if len(tagsJSON) > 0 {
		_ = json.Unmarshal(tagsJSON, &p.Tags)
	}
	if clientID != nil {
		p.Info = &centrifuge.ClientInfo{
			ClientID: *clientID,
			ConnInfo: connInfo,
			ChanInfo: chanInfo,
		}
		if userID != nil {
			p.Info.UserID = *userID
		}
	}
	return centrifuge.MapStateResult{Publications: []*centrifuge.Publication{&p}, Position: streamPos}, nil
}

// ReadStream retrieves publications from stream.
func (e *PostgresMapBroker) ReadStream(ctx context.Context, ch string, opts centrifuge.MapReadStreamOptions) (centrifuge.MapStreamResult, error) {
	pool := e.getReadPool(ch, opts.AllowCached)

	if opts.Filter.Limit == 0 {
		// Position check only — single meta query, no stream read needed.
		return e.readStreamPosition(ctx, pool, ch)
	}

	sinceOffset := int64(0)
	if opts.Filter.Since != nil {
		sinceOffset = int64(opts.Filter.Since.Offset)
	}

	limit := opts.Filter.Limit
	unlimited := limit < 0

	// Build stream query.
	streamTable := e.names.stream
	var streamQuery string
	if opts.Filter.Reverse {
		if opts.Filter.Since == nil {
			// For reverse without explicit Since, we need topOffset from meta.
			// We handle this after reading meta from the batch result.
			sinceOffset = 0 // placeholder, will be overridden
		}
		if unlimited {
			streamQuery = fmt.Sprintf(`
				SELECT key, data, tags, channel_offset, removed, score, client_id, user_id, conn_info, chan_info
				FROM %s
				WHERE channel = $1 AND channel_offset < $2
				ORDER BY channel_offset DESC
			`, streamTable)
		} else {
			streamQuery = fmt.Sprintf(`
				SELECT key, data, tags, channel_offset, removed, score, client_id, user_id, conn_info, chan_info
				FROM %s
				WHERE channel = $1 AND channel_offset < $2
				ORDER BY channel_offset DESC
				LIMIT $3
			`, streamTable)
		}
	} else {
		if unlimited {
			streamQuery = fmt.Sprintf(`
				SELECT key, data, tags, channel_offset, removed, score, client_id, user_id, conn_info, chan_info
				FROM %s
				WHERE channel = $1 AND channel_offset > $2
				ORDER BY channel_offset ASC
			`, streamTable)
		} else {
			streamQuery = fmt.Sprintf(`
				SELECT key, data, tags, channel_offset, removed, score, client_id, user_id, conn_info, chan_info
				FROM %s
				WHERE channel = $1 AND channel_offset > $2
				ORDER BY channel_offset ASC
				LIMIT $3
			`, streamTable)
		}
	}

	// For reverse without Since, we need topOffset to set sinceOffset.
	// Fall back to transactional path for this case.
	if opts.Filter.Reverse && opts.Filter.Since == nil {
		return e.readStreamTx(ctx, pool, ch, opts, streamQuery, unlimited, limit)
	}

	// Pipelined batch: meta + stream in a single round trip, with REPEATABLE READ for consistency.
	metaQuery := fmt.Sprintf(`SELECT top_offset, epoch FROM %s WHERE channel = $1`, e.names.meta)
	batch := &pgx.Batch{}
	batch.Queue("BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY")
	batch.Queue(metaQuery, ch)
	if unlimited {
		batch.Queue(streamQuery, ch, sinceOffset)
	} else {
		batch.Queue(streamQuery, ch, sinceOffset, limit)
	}
	batch.Queue("COMMIT")

	br := pool.SendBatch(ctx, batch)

	// Consume BEGIN.
	if _, err := br.Exec(); err != nil {
		_ = br.Close()
		return centrifuge.MapStreamResult{}, err
	}

	// Read meta.
	var topOffset int64
	var epoch string
	err := br.QueryRow().Scan(&topOffset, &epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = br.Close()
		return centrifuge.MapStreamResult{}, nil
	}
	if err != nil {
		_ = br.Close()
		return centrifuge.MapStreamResult{}, err
	}

	streamPos := centrifuge.StreamPosition{Offset: uint64(topOffset), Epoch: epoch}

	// Validate epoch if provided.
	if opts.Filter.Since != nil && opts.Filter.Since.Epoch != "" && opts.Filter.Since.Epoch != epoch {
		_ = br.Close()
		return centrifuge.MapStreamResult{}, centrifuge.ErrorUnrecoverablePosition
	}

	// Read stream rows.
	rows, err := br.Query()
	if err != nil {
		_ = br.Close()
		return centrifuge.MapStreamResult{}, err
	}

	allocHint := limit
	if unlimited {
		allocHint = 64
	}
	arena := byteArena{buf: make([]byte, 0, allocHint*64)}
	backing := make([]centrifuge.Publication, 0, allocHint)
	pubs := make([]*centrifuge.Publication, 0, allocHint)
	// Column order: key(0), data(1), tags(2), channel_offset(3), removed(4),
	//               score(5), client_id(6), user_id(7), conn_info(8), chan_info(9).
	var fmts pgColFormats
	for rows.Next() {
		if fmts == nil {
			fmts = pgColFormatsFromRows(rows)
		}
		raw := rows.RawValues()
		backing = append(backing, centrifuge.Publication{})
		p := &backing[len(backing)-1]
		p.Key = pgRawString(&arena, raw[0])
		p.Data = e.rawDataBytes(&arena, raw[1], fmts[1])
		p.Tags = pgRawJSONBMap(raw[2])
		p.Offset = pgRawUint64(raw[3], fmts[3])
		p.Removed = pgRawBool(raw[4], fmts[4])
		p.Score = pgRawInt64(raw[5], fmts[5])
		if raw[6] != nil {
			p.Info = &centrifuge.ClientInfo{
				ClientID: pgRawString(&arena, raw[6]),
				UserID:   pgRawString(&arena, raw[7]),
				ConnInfo: e.rawDataBytes(&arena, raw[8], fmts[8]),
				ChanInfo: e.rawDataBytes(&arena, raw[9], fmts[9]),
			}
		}
		pubs = append(pubs, p)
	}
	rows.Close()

	// Consume COMMIT.
	_, _ = br.Exec()
	_ = br.Close()

	if err := rows.Err(); err != nil {
		return centrifuge.MapStreamResult{}, err
	}

	return centrifuge.MapStreamResult{Publications: pubs, Position: streamPos}, nil
}

// readStreamTx is a fallback for ReadStream when we need meta before building the query
// (e.g., reverse without explicit Since needs topOffset). Uses REPEATABLE READ transaction.
func (e *PostgresMapBroker) readStreamTx(ctx context.Context, pool *pgxpool.Pool, ch string, _ centrifuge.MapReadStreamOptions, streamQuery string, unlimited bool, limit int) (centrifuge.MapStreamResult, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return centrifuge.MapStreamResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var topOffset int64
	var epoch string
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT top_offset, epoch FROM %s WHERE channel = $1
	`, e.names.meta), ch).Scan(&topOffset, &epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return centrifuge.MapStreamResult{}, nil
	}
	if err != nil {
		return centrifuge.MapStreamResult{}, err
	}

	sinceOffset := topOffset + 1

	var rows pgx.Rows
	if unlimited {
		rows, err = tx.Query(ctx, streamQuery, ch, sinceOffset)
	} else {
		rows, err = tx.Query(ctx, streamQuery, ch, sinceOffset, limit)
	}
	if err != nil {
		return centrifuge.MapStreamResult{}, err
	}
	defer rows.Close()

	allocHint := limit
	if unlimited {
		allocHint = 64
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
		p.Key = pgRawString(&arena, raw[0])
		p.Data = e.rawDataBytes(&arena, raw[1], fmts[1])
		p.Tags = pgRawJSONBMap(raw[2])
		p.Offset = pgRawUint64(raw[3], fmts[3])
		p.Removed = pgRawBool(raw[4], fmts[4])
		p.Score = pgRawInt64(raw[5], fmts[5])
		if raw[6] != nil {
			p.Info = &centrifuge.ClientInfo{
				ClientID: pgRawString(&arena, raw[6]),
				UserID:   pgRawString(&arena, raw[7]),
				ConnInfo: e.rawDataBytes(&arena, raw[8], fmts[8]),
				ChanInfo: e.rawDataBytes(&arena, raw[9], fmts[9]),
			}
		}
		pubs = append(pubs, p)
	}
	if err := rows.Err(); err != nil {
		return centrifuge.MapStreamResult{}, err
	}

	return centrifuge.MapStreamResult{Publications: pubs, Position: centrifuge.StreamPosition{Offset: uint64(topOffset), Epoch: epoch}}, nil
}

// readStreamPosition returns just the stream position for a channel (Limit=0 case).
func (e *PostgresMapBroker) readStreamPosition(ctx context.Context, pool *pgxpool.Pool, ch string) (centrifuge.MapStreamResult, error) {
	var topOffset int64
	var epoch string
	err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT top_offset, epoch FROM %s WHERE channel = $1
	`, e.names.meta), ch).Scan(&topOffset, &epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		return centrifuge.MapStreamResult{}, nil
	}
	if err != nil {
		return centrifuge.MapStreamResult{}, err
	}
	return centrifuge.MapStreamResult{Position: centrifuge.StreamPosition{Offset: uint64(topOffset), Epoch: epoch}}, nil
}

// Stats returns state statistics.
func (e *PostgresMapBroker) Stats(ctx context.Context, ch string) (centrifuge.MapStats, error) {
	pool := e.getReadPool(ch, false)

	var count int
	err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM %s
		WHERE channel = $1
	`, e.names.state), ch).Scan(&count)
	if err != nil {
		return centrifuge.MapStats{}, err
	}

	return centrifuge.MapStats{NumKeys: count}, nil
}

// Clear deletes all data for a channel.
func (e *PostgresMapBroker) Clear(ctx context.Context, ch string, _ centrifuge.MapClearOptions) error {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE channel = $1`, e.names.stream), ch)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE channel = $1`, e.names.state), ch)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE channel = $1`, e.names.meta), ch)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE channel = $1`, e.names.idempotency), ch)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ============================================================================
// Notification Listener (optional, only started when UseNotify=true)
// ============================================================================

// runNotificationListener listens for pg_notify and wakes the outbox worker.
func (e *PostgresMapBroker) runNotificationListener() {
	ctx := e.cancelCtx
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-e.closeCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		conn, err := e.pool.Acquire(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			e.logErrorMsg("notification listener: acquire connection", err)
			time.Sleep(backoff)
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		_, err = conn.Exec(ctx, "LISTEN "+e.names.notifyChannel)
		if err != nil {
			conn.Release()
			if ctx.Err() != nil {
				return
			}
			e.logErrorMsg("notification listener: LISTEN", err)
			time.Sleep(backoff)
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		backoff = time.Second

		// Notification loop
		for {
			_, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				conn.Release()
				if ctx.Err() != nil {
					return
				}
				e.logErrorMsg("notification listener: wait", err)
				break // reconnect
			}

			// Non-blocking send to wake outbox worker
			select {
			case e.notifyCh <- struct{}{}:
			default:
			}
		}

		select {
		case <-e.closeCh:
			return
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

// ============================================================================
// Outbox Worker Implementation
// ============================================================================

// outboxWorkerConfig returns (pool, shardIDs) for outbox worker #workerIdx.
// Each worker handles exactly one shard. With replicas, the worker reads from
// readPools[workerIdx % len(readPools)], matching getReadPool's routing.
func (e *PostgresMapBroker) outboxWorkerConfig(workerIdx int) (*pgxpool.Pool, []int) {
	pool := e.pool
	if len(e.readPools) > 0 {
		pool = e.readPools[workerIdx%len(e.readPools)]
	}
	return pool, []int{workerIdx}
}

// runOutboxWorker polls the stream table for new entries and delivers them.
//
// Per-shard serialization (FOR UPDATE on shard_lock) combined with
// one-shard-per-worker guarantees that BIGSERIAL IDs within a shard are
// committed in order — no gaps possible. This allows a simple maxID cursor.
func (e *PostgresMapBroker) runOutboxWorker(workerIdx int) {
	ctx := e.cancelCtx
	pool, shards := e.outboxWorkerConfig(workerIdx)

	// Initialize cursor: start from current max ID so we only get future entries.
	var cursor int64
	err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT COALESCE(MAX(id), 0) FROM %s`, e.names.stream)).Scan(&cursor)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		e.logErrorMsg("outbox worker: init cursor", err)
		// Retry after delay
		time.Sleep(time.Second)
		go e.runOutboxWorker(workerIdx)
		return
	}

	pollInterval := e.conf.Outbox.PollInterval

	// Pre-allocate reusable batch buffer.
	allocHint := e.conf.Outbox.BatchSize
	if allocHint > 1001 {
		allocHint = 1001
	}
	buf := &outboxBatchBuf{
		metas: make([]outboxMeta, 0, allocHint),
	}

	for {
		select {
		case <-e.closeCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		// Deliver batches until idle.
		idle := true
		for {
			processed, maxID, err := e.processOutboxBatch(ctx, pool, cursor, shards, buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				e.logErrorMsg("outbox worker: process batch", err)
				break
			}
			if processed == 0 {
				break
			}
			if maxID > cursor {
				cursor = maxID
			}
			idle = false
		}

		// Wait for notification or poll interval (only when idle).
		if !idle {
			continue
		}
		if e.notifyCh != nil {
			select {
			case <-e.closeCh:
				return
			case <-ctx.Done():
				return
			case <-e.notifyCh:
			case <-time.After(pollInterval):
			}
		} else {
			select {
			case <-e.closeCh:
				return
			case <-ctx.Done():
				return
			case <-time.After(pollInterval):
			}
		}
	}
}

// runOutboxWorkerWithLock uses PostgreSQL advisory locks to ensure only one node
// per shard polls the stream table. Used when Broker fan-out is enabled.
// The worker acquires a session-level advisory lock on the primary pool.
// If the lock is held by another node, it retries after AdvisoryLockRetryInterval.
// Once acquired, it runs the normal outbox poll loop. The lock is automatically
// released when the connection is returned to the pool or dropped.
func (e *PostgresMapBroker) runOutboxWorkerWithLock(workerIdx int) {
	ctx := e.cancelCtx
	lockID := e.conf.Outbox.AdvisoryLockBaseID + int64(workerIdx)
	retryInterval := e.conf.Outbox.AdvisoryLockRetryInterval
	_, shards := e.outboxWorkerConfig(workerIdx)

	for {
		select {
		case <-e.closeCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		// Acquire a connection from the primary pool for the advisory lock.
		conn, err := e.pool.Acquire(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			e.logError("outbox worker: acquire lock connection", err, workerIdx)
			select {
			case <-e.closeCh:
				return
			case <-ctx.Done():
				return
			case <-time.After(retryInterval):
			}
			continue
		}

		// Try to acquire advisory lock (non-blocking).
		var locked bool
		err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&locked)
		if err != nil {
			conn.Release()
			if ctx.Err() != nil {
				return
			}
			e.logError("outbox worker: try advisory lock", err, workerIdx)
			select {
			case <-e.closeCh:
				return
			case <-ctx.Done():
				return
			case <-time.After(retryInterval):
			}
			continue
		}

		if !locked {
			conn.Release()
			select {
			case <-e.closeCh:
				return
			case <-ctx.Done():
				return
			case <-time.After(retryInterval):
			}
			continue
		}

		e.logInfo("outbox worker: acquired advisory lock", workerIdx)

		// Lock acquired — run the poll loop.
		// Use read pool (replica) for batch polling to offload the primary.
		pollPool, _ := e.outboxWorkerConfig(workerIdx)

		// Initialize cursor from current max ID.
		var cursor int64
		err = pollPool.QueryRow(ctx, fmt.Sprintf(
			`SELECT COALESCE(MAX(id), 0) FROM %s`, e.names.stream)).Scan(&cursor)
		if err != nil {
			e.releaseAdvisoryLock(ctx, conn, lockID, workerIdx)
			conn.Release()
			if ctx.Err() != nil {
				return
			}
			e.logError("outbox worker: init cursor", err, workerIdx)
			select {
			case <-e.closeCh:
				return
			case <-ctx.Done():
				return
			case <-time.After(retryInterval):
			}
			continue
		}

		pollInterval := e.conf.Outbox.PollInterval
		allocHint := e.conf.Outbox.BatchSize
		if allocHint > 1001 {
			allocHint = 1001
		}
		buf := &outboxBatchBuf{
			metas: make([]outboxMeta, 0, allocHint),
		}

		// Poll loop — runs until error or shutdown.
		lockLost := false
		for !lockLost {
			select {
			case <-e.closeCh:
				e.releaseAdvisoryLock(ctx, conn, lockID, workerIdx)
				conn.Release()
				return
			case <-ctx.Done():
				e.releaseAdvisoryLock(ctx, conn, lockID, workerIdx)
				conn.Release()
				return
			default:
			}

			idle := true
			for {
				processed, maxID, err := e.processOutboxBatch(ctx, pollPool, cursor, shards, buf)
				if err != nil {
					if ctx.Err() != nil {
						e.releaseAdvisoryLock(ctx, conn, lockID, workerIdx)
						conn.Release()
						return
					}
					e.logError("outbox worker: process batch", err, workerIdx)
					lockLost = true
					break
				}
				if processed == 0 {
					break
				}
				if maxID > cursor {
					cursor = maxID
				}
				idle = false
			}

			if lockLost {
				break
			}

			if !idle {
				continue
			}
			select {
			case <-e.closeCh:
				e.releaseAdvisoryLock(ctx, conn, lockID, workerIdx)
				conn.Release()
				return
			case <-ctx.Done():
				e.releaseAdvisoryLock(ctx, conn, lockID, workerIdx)
				conn.Release()
				return
			case <-time.After(pollInterval):
			}
		}

		e.releaseAdvisoryLock(ctx, conn, lockID, workerIdx)
		conn.Release()

		// Back off before retrying lock acquisition.
		select {
		case <-e.closeCh:
			return
		case <-ctx.Done():
			return
		case <-time.After(retryInterval):
		}
	}
}

// releaseAdvisoryLock explicitly releases an advisory lock. This is best-effort;
// the lock is also auto-released when the connection is returned to the pool.
func (e *PostgresMapBroker) releaseAdvisoryLock(ctx context.Context, conn *pgxpool.Conn, lockID int64, shardID int) {
	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := conn.Exec(releaseCtx, "SELECT pg_advisory_unlock($1)", lockID)
	if err != nil && ctx.Err() == nil {
		e.logError("outbox worker: release advisory lock", err, shardID)
	}
}

// outboxMeta holds per-row metadata not captured in Publication.
// String/byte fields reference arena memory — no per-field heap allocation.
type outboxMeta struct {
	id           int64
	channel      string
	epoch        string
	previousData []byte
}

// outboxBatchBuf holds reusable buffers for processOutboxBatch. Only metas
// (internal metadata) is reused across batches. pubBacking and infoBacking are
// allocated fresh each batch because handlers may retain *Publication pointers
// asynchronously (e.g. cachedEventHandler.BufferPublication, channelMedium queue).
type outboxBatchBuf struct {
	metas []outboxMeta
}

// reset clears metas while retaining capacity.
func (b *outboxBatchBuf) reset() {
	clear(b.metas[:cap(b.metas)])
	b.metas = b.metas[:0]
}

// processOutboxBatch fetches and processes a batch of stream entries for the
// given shards. Per-shard serialization (FOR UPDATE on shard_lock) guarantees
// IDs within a shard are committed in order, so a simple cursor is safe.
// Uses RawValues + byteArena to avoid per-row heap allocations from pgx Scan.
func (e *PostgresMapBroker) processOutboxBatch(ctx context.Context, pool *pgxpool.Pool, cursor int64, shardIDs []int, buf *outboxBatchBuf) (int, int64, error) {
	batchSize := e.conf.Outbox.BatchSize

	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT id, shard_id, channel, channel_offset, epoch, key, data, tags, removed, score,
			   client_id, user_id, conn_info, chan_info, previous_data, created_at
		FROM %s
		WHERE id > $1 AND shard_id = ANY($2)
		ORDER BY id
		LIMIT $3
	`, e.names.stream), cursor, shardIDs, batchSize)
	if err != nil {
		return 0, cursor, fmt.Errorf("query stream: %w", err)
	}

	buf.reset()
	arena := byteArena{}
	allocHint := batchSize
	if allocHint > 1001 {
		allocHint = 1001
	}
	pubBacking := make([]centrifuge.Publication, 0, allocHint)
	infoBacking := make([]centrifuge.ClientInfo, 0, allocHint/4+1)

	var maxID int64

	// Use RawValues + arena to avoid per-row allocations.
	// Column order: id(0), shard_id(1), channel(2), channel_offset(3), epoch(4),
	//              key(5), data(6), tags(7), removed(8), score(9),
	//              client_id(10), user_id(11), conn_info(12), chan_info(13),
	//              previous_data(14), created_at(15).
	var fmts pgColFormats
	for rows.Next() {
		if fmts == nil {
			fmts = pgColFormatsFromRows(rows)
		}
		raw := rows.RawValues()

		id := pgRawInt64(raw[0], fmts[0])
		if id > maxID {
			maxID = id
		}

		pubBacking = append(pubBacking, centrifuge.Publication{})
		p := &pubBacking[len(pubBacking)-1]

		p.Offset = pgRawUint64(raw[3], fmts[3])
		p.Key = pgRawString(&arena, raw[5])
		p.Data = e.rawDataBytes(&arena, raw[6], fmts[6])
		p.Tags = pgRawJSONBMap(raw[7])
		p.Removed = pgRawBool(raw[8], fmts[8])
		p.Score = pgRawInt64(raw[9], fmts[9])
		p.Time = pgRawTimestampMillis(raw[15], fmts[15])
		if raw[10] != nil {
			infoBacking = append(infoBacking, centrifuge.ClientInfo{
				ClientID: pgRawString(&arena, raw[10]),
				UserID:   pgRawString(&arena, raw[11]),
				ConnInfo: e.rawDataBytes(&arena, raw[12], fmts[12]),
				ChanInfo: e.rawDataBytes(&arena, raw[13], fmts[13]),
			})
			p.Info = &infoBacking[len(infoBacking)-1]
		}

		m := outboxMeta{
			id:           id,
			channel:      pgRawString(&arena, raw[2]),
			epoch:        pgRawString(&arena, raw[4]),
			previousData: e.rawDataBytes(&arena, raw[14], fmts[14]),
		}
		buf.metas = append(buf.metas, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, cursor, fmt.Errorf("iterate rows: %w", err)
	}

	if len(buf.metas) == 0 {
		return 0, cursor, nil
	}

	// Deliver each entry.
	for i := range buf.metas {
		m := &buf.metas[i]
		pub := &pubBacking[i]
		streamPos := centrifuge.StreamPosition{Offset: pub.Offset, Epoch: m.epoch}

		var prevPub *centrifuge.Publication
		useDelta := len(m.previousData) > 0
		if useDelta {
			prevPub = &centrifuge.Publication{Data: m.previousData}
		}

		if e.conf.Broker != nil {
			// Fan-out via inner Broker (e.g. Redis PUB/SUB).
			pubOpts := centrifuge.PublishOptions{
				ClientInfo: pub.Info,
				Key:        pub.Key,
				Removed:    pub.Removed,
				Score:      pub.Score,
				Tags:       pub.Tags,
				Offset:     pub.Offset,
				Epoch:      m.epoch,
				UseDelta:   useDelta,
			}
			if useDelta {
				pubOpts.PrevData = m.previousData
			}
			if _, err := e.conf.Broker.Publish(m.channel, pub.Data, pubOpts); err != nil {
				e.logErrorMsg("outbox worker: broker publish", err)
			}
		} else if e.eventHandler != nil {
			_ = e.eventHandler.HandlePublication(m.channel, pub, streamPos, useDelta, prevPub)
		}
	}

	return len(buf.metas), maxID, nil
}

func (e *PostgresMapBroker) logEvent() *zerolog.Event {
	ev := log.Error()
	if e.conf.Name != "" {
		ev = ev.Str("broker_name", e.conf.Name)
	}
	return ev
}

func (e *PostgresMapBroker) logErrorMsg(msg string, err error) {
	e.logEvent().Err(err).Msg(msg)
}

func (e *PostgresMapBroker) logError(msg string, err error, shardID int) {
	e.logEvent().Err(err).Int("shard", shardID).Msg(msg)
}

func (e *PostgresMapBroker) logInfo(msg string, shardID int) {
	ev := log.Info()
	if e.conf.Name != "" {
		ev = ev.Str("broker_name", e.conf.Name)
	}
	ev.Int("shard", shardID).Msg(msg)
}

// ============================================================================
// TTL Expiration Worker
// ============================================================================

// runTTLExpirationWorker expires keys with TTL and emits removal events.
func (e *PostgresMapBroker) runTTLExpirationWorker() {
	ticker := time.NewTicker(e.conf.TTLCheckInterval)
	defer ticker.Stop()
	ctx := e.cancelCtx

	for {
		select {
		case <-e.closeCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.expireKeys(ctx)
		}
	}
}

func (e *PostgresMapBroker) expireKeys(ctx context.Context) {
	numShards := e.conf.NumShards

	// Find distinct channels with expired keys to resolve per-channel options.
	channelRows, err := e.pool.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT channel FROM %s
		WHERE expires_at IS NOT NULL AND expires_at <= NOW()
		LIMIT 100
	`, e.names.state))
	if err != nil {
		e.logErrorMsg("error querying channels for key expiration", err)
		e.node.IncMapBrokerCleanupErrors(e.conf.Name)
		return
	}
	var channels []string
	for channelRows.Next() {
		var ch string
		if err := channelRows.Scan(&ch); err != nil {
			continue
		}
		channels = append(channels, ch)
	}
	channelRows.Close()

	// Process each channel with its own resolved options.
	for _, ch := range channels {
		chOpts, err := centrifuge.ResolveAndValidateMapChannelOptions(e.node.Config().GetMapChannelOptions, ch)
		if err != nil {
			e.logEvent().Err(err).Str("channel", ch).Msg("error resolving channel options for key expiration")
			e.node.IncMapBrokerCleanupErrors(e.conf.Name)
			continue
		}
		var metaTTL *string
		if chOpts.MetaTTL > 0 {
			s := durationToIntervalString(chOpts.MetaTTL)
			metaTTL = &s
		}

		// Call expire_keys SQL function which atomically:
		// 1. Deletes expired keys from state
		// 2. Inserts removal events into the stream
		// The outbox worker will pick up the stream entries and deliver them
		// via HandlePublication — we must NOT call HandlePublication here to avoid
		// duplicate delivery.
		rows, err := e.pool.Query(ctx, fmt.Sprintf(`
			SELECT out_channel, out_key, out_offset, out_epoch
			FROM %s($1, $2, $3::interval, $4, $5)
		`, e.names.expireKeys), 1000, numShards, metaTTL, ch, e.conf.SkipShardLock)
		if err != nil {
			e.logEvent().Err(err).Str("channel", ch).Msg("error in batch key expiration")
			e.node.IncMapBrokerCleanupErrors(e.conf.Name)
			continue
		}
		// Count rows to track keys removed.
		var removedCount int64
		for rows.Next() {
			removedCount++
		}
		rows.Close()
		if removedCount > 0 {
			e.node.AddMapBrokerCleanupKeysRemoved(e.conf.Name, removedCount)
		}
	}
}

// runCleanupLagWorker periodically checks the oldest expired entry in the state
// table and reports it as a lag metric. This runs in a separate goroutine to
// avoid adding extra queries to the hot cleanup path.
func (e *PostgresMapBroker) runCleanupLagWorker() {
	interval := e.conf.TTLCheckInterval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	ctx := e.cancelCtx

	for {
		select {
		case <-e.closeCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.updateCleanupLag(ctx)
		}
	}
}

func (e *PostgresMapBroker) updateCleanupLag(ctx context.Context) {
	var expiresAt *time.Time
	err := e.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT MIN(expires_at) FROM %s
		WHERE expires_at IS NOT NULL AND expires_at <= NOW()
	`, e.names.state)).Scan(&expiresAt)
	if err != nil || expiresAt == nil {
		e.node.SetMapBrokerCleanupLag(e.conf.Name, 0)
		return
	}
	lagSeconds := time.Since(*expiresAt).Seconds()
	if lagSeconds < 0 {
		lagSeconds = 0
	}
	e.node.SetMapBrokerCleanupLag(e.conf.Name, lagSeconds)
}

// ============================================================================
// Cleanup Worker
// ============================================================================

// runCleanupWorker cleans up old stream entries and expired meta/idempotency entries.
func (e *PostgresMapBroker) runCleanupWorker() {
	ticker := time.NewTicker(e.conf.CleanupInterval)
	defer ticker.Stop()
	ctx := e.cancelCtx

	for {
		select {
		case <-e.closeCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.cleanupEntries(ctx)
		}
	}
}

func (e *PostgresMapBroker) cleanupEntries(ctx context.Context) {
	// Remove old stream entries (time-based retention).
	if !e.conf.Partitioning {
		retention := durationToIntervalString(e.conf.StreamRetention)
		if _, err := e.pool.Exec(ctx, fmt.Sprintf(`
			DELETE FROM %s
			WHERE created_at < NOW() - $1::interval
		`, e.names.stream), retention); err != nil {
			e.logErrorMsg("error cleaning up old stream entries", err)
		}
	}

	// Remove expired stream metadata
	if _, err := e.pool.Exec(ctx, fmt.Sprintf(`
		DELETE FROM %s
		WHERE expires_at IS NOT NULL AND expires_at < NOW()
	`, e.names.meta)); err != nil {
		e.logErrorMsg("error cleaning up expired stream metadata", err)
	}

	// Remove expired idempotency keys
	if _, err := e.pool.Exec(ctx, fmt.Sprintf(`
		DELETE FROM %s
		WHERE expires_at < NOW()
	`, e.names.idempotency)); err != nil {
		e.logErrorMsg("error cleaning up expired idempotency keys", err)
	}
}

// ============================================================================
// Partitioning Support (optional optimization)
// ============================================================================

// ensurePartitionedStream converts the stream table to partitioned if needed.
func (e *PostgresMapBroker) ensurePartitionedStream(ctx context.Context) error {
	// Check if table is already partitioned
	var isPartitioned bool
	err := e.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_partitioned_table
			WHERE partrelid = $1::regclass
		)
	`, e.names.stream).Scan(&isPartitioned)
	if err != nil {
		// If the table doesn't exist as partitioned, we need to set it up
		isPartitioned = false
	}

	if !isPartitioned {
		// Drop and recreate as partitioned table (only safe for fresh DB).
		// For existing deployments, manual migration would be needed.
		_, err := e.pool.Exec(ctx, fmt.Sprintf(`
			DROP TABLE IF EXISTS %s CASCADE;
			CREATE TABLE %s (
				id              BIGSERIAL,
				channel         TEXT NOT NULL,
				channel_offset  BIGINT NOT NULL,
				epoch           TEXT NOT NULL DEFAULT '',
				key             TEXT NOT NULL,
				data            %s,
				tags            JSONB,
				client_id       TEXT,
				user_id         TEXT,
				conn_info       %s,
				chan_info        %s,
				subscribed_at   TIMESTAMPTZ,
				removed         BOOLEAN DEFAULT FALSE,
				score           BIGINT,
				previous_data   %s,
				created_at      TIMESTAMPTZ DEFAULT NOW(),
				shard_id        SMALLINT NOT NULL DEFAULT 0,
				PRIMARY KEY (id, created_at)
			) PARTITION BY RANGE (created_at);
			CREATE INDEX IF NOT EXISTS %s_channel_offset_idx ON %s (channel, channel_offset);
			CREATE INDEX IF NOT EXISTS %s_channel_id_idx ON %s (channel, id DESC);
			CREATE INDEX IF NOT EXISTS %s_shard_id_idx ON %s (shard_id, id);
		`, e.names.stream, e.names.stream,
			e.dataType(), e.dataType(), e.dataType(), e.dataType(),
			e.names.stream, e.names.stream,
			e.names.stream, e.names.stream,
			e.names.stream, e.names.stream,
		))
		if err != nil {
			return &SchemaError{
				Object: SchemaObject{Type: "table", Name: e.names.stream},
				Op:     "create",
				Err:    fmt.Errorf("create partitioned stream: %w", err),
			}
		}

		// Re-create the publish/remove/expire functions since they reference the stream table.
		schemaSQL := postgresSchemaJSONBSQL
		if e.conf.BinaryData {
			schemaSQL = postgresSchemaBinarySQL
		}
		// Extract only the function definitions (skip CREATE TABLE IF NOT EXISTS lines).
		if _, err := e.pool.Exec(ctx, schemaSQL); err != nil {
			return &SchemaError{
				Object: SchemaObject{Type: "function", Name: ""},
				Op:     "create",
				Err:    fmt.Errorf("recreate functions after partitioning: %w", err),
			}
		}
	}

	// Ensure lookahead partitions exist
	return e.ensureLookaheadPartitions(ctx)
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

// rawDataBytes reads a data column (JSONB or BYTEA depending on BinaryData config)
// using the correct format-aware parser.
func (e *PostgresMapBroker) rawDataBytes(a *byteArena, b []byte, format int16) []byte {
	if e.conf.BinaryData {
		return pgRawBytes(a, b, format)
	}
	return pgRawJSONBBytes(a, b, format)
}

// dataParam wraps a []byte for use as a SQL parameter.
// When BinaryData is false, data columns are JSONB — wrap as json.RawMessage
// so pgx encodes it as JSON text (not hex bytea) in simple-protocol mode.
// When BinaryData is true, data columns are BYTEA — pass as plain []byte.
func (e *PostgresMapBroker) dataParam(b []byte) any {
	if b == nil {
		return nil
	}
	if !e.conf.BinaryData {
		return json.RawMessage(b)
	}
	return b
}

func (e *PostgresMapBroker) dataType() string {
	if e.conf.BinaryData {
		return "BYTEA"
	}
	return "JSONB"
}

func (e *PostgresMapBroker) ensureLookaheadPartitions(ctx context.Context) error {
	now := time.Now().UTC()
	for d := 0; d <= e.conf.PartitionLookaheadDays; d++ {
		day := now.AddDate(0, 0, d)
		nextDay := day.AddDate(0, 0, 1)
		partName := fmt.Sprintf("%s_%s", e.names.stream, day.Format("2006_01_02"))
		_, err := e.pool.Exec(ctx, fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
			partName, e.names.stream,
			day.Format("2006-01-02"), nextDay.Format("2006-01-02"),
		))
		if err != nil {
			return &SchemaError{
				Object: SchemaObject{Type: "table", Name: partName},
				Op:     "create",
				Err:    err,
			}
		}
	}
	return nil
}

func (e *PostgresMapBroker) dropOldPartitions(ctx context.Context) {
	cutoff := time.Now().UTC().AddDate(0, 0, -e.conf.PartitionRetentionDays)

	// List child partitions via pg_inherits
	rows, err := e.pool.Query(ctx, `
		SELECT c.relname
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname = $1
	`, e.names.stream)
	if err != nil {
		e.logErrorMsg("error listing partitions for cleanup", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var partName string
		if err := rows.Scan(&partName); err != nil {
			continue
		}
		// Parse date from partition name suffix: {stream}_{YYYY}_{MM}_{DD}
		parts := strings.Split(partName, "_")
		if len(parts) < 3 {
			continue
		}
		dateStr := strings.Join(parts[len(parts)-3:], "-")
		partDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		if partDate.Before(cutoff) {
			_, err := e.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", partName))
			if err != nil {
				e.logErrorMsg("error dropping old partition "+partName, err)
			}
		}
	}
}

// runPartitionWorker manages partition creation and cleanup.
func (e *PostgresMapBroker) runPartitionWorker() {
	ticker := time.NewTicker(e.conf.CleanupInterval)
	defer ticker.Stop()
	ctx := e.cancelCtx

	for {
		select {
		case <-e.closeCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.ensureLookaheadPartitions(ctx); err != nil {
				e.logErrorMsg("error ensuring lookahead partitions", err)
			}
			e.dropOldPartitions(ctx)
		}
	}
}
