package controllers

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/pgoutbox"

	"github.com/centrifugal/centrifuge"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

//go:embed controller_postgres_schema.sql
var postgresControllerSchemaTemplate string

var controllerSchemaVersion = 1

// PostgresControllerConfig configures the PostgreSQL controller.
type PostgresControllerConfig struct {
	// DSN is the primary PostgreSQL connection string.
	DSN string

	// TLS is an optional TLS configuration applied to all pools.
	TLS configtypes.TLSConfig
	// PoolSize sets the maximum number of connections in the primary pool.
	// Default: 8.
	PoolSize int
	// NumShards is the number of shards for serialized publishing. Each shard
	// has a lock row; the publish function acquires FOR UPDATE before INSERT,
	// ensuring BIGSERIAL ids are assigned in commit order within a shard.
	// Default: 1 (single shard — sufficient for controller traffic).
	NumShards int
	// TablePrefix is the user-facing namespace prefix. Default "cf".
	// Produces table names like cf_controller_messages.
	TablePrefix string
	// PollInterval is how often to poll for new control messages when idle.
	// Default: 50ms.
	PollInterval time.Duration
	// UseNotify enables LISTEN/NOTIFY for low-latency wakeup.
	// Default: true.
	UseNotify bool
	// NotifyDSN is an optional separate connection string used exclusively for
	// the LISTEN connection when UseNotify is true. Set this to a direct
	// PostgreSQL URL (bypassing PGBouncer) when DSN points at a PGBouncer
	// endpoint — PGBouncer transaction pooling mode is incompatible with
	// LISTEN/NOTIFY. If empty, the primary DSN pool is used.
	NotifyDSN string
	// PartitionRetentionDays controls how old a partition must be before
	// it is dropped. Default: 1 (control messages are ephemeral).
	PartitionRetentionDays int
	// PartitionLookaheadDays controls how many future daily partitions
	// to pre-create. Default: 2.
	PartitionLookaheadDays int
	// PartitionCleanupInterval is the ticker interval for the partitioner.
	// Default: 1m.
	PartitionCleanupInterval time.Duration
	// BatchSize is the maximum number of rows to process per batch.
	// Default: 1000.
	BatchSize int
}

func (c *PostgresControllerConfig) setDefaults() {
	if c.PoolSize <= 0 {
		c.PoolSize = 8
	}
	if c.NumShards <= 0 {
		c.NumShards = 1
	}
	if c.TablePrefix == "" {
		c.TablePrefix = "cf"
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 50 * time.Millisecond
	}
	if c.PartitionLookaheadDays <= 0 {
		c.PartitionLookaheadDays = 2
	}
	if c.PartitionRetentionDays <= 0 {
		c.PartitionRetentionDays = 1
	}
	if c.PartitionCleanupInterval <= 0 {
		c.PartitionCleanupInterval = time.Minute
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 1000
	}
}

// controllerNames holds precomputed table/function/channel names.
type controllerNames struct {
	messages      string // e.g. cf_controller_messages
	shardLock     string // e.g. cf_controller_shard_lock
	schemaVersion string // e.g. cf_controller_schema_version
	publishFunc   string // e.g. cf_controller_publish
	notifyChannel string // e.g. cf_controller_notify
}

func newControllerNames(prefix string) controllerNames {
	// Normalize: ensure prefix ends with "_".
	p := strings.TrimRight(prefix, "_") + "_"
	return controllerNames{
		messages:      p + "controller_messages",
		shardLock:     p + "controller_shard_lock",
		schemaVersion: p + "controller_schema_version",
		publishFunc:   p + "controller_publish",
		notifyChannel: p + "controller_notify",
	}
}

// PostgresController implements centrifuge.Controller using PostgreSQL.
type PostgresController struct {
	node         *centrifuge.Node
	conf         PostgresControllerConfig
	pool         *pgxpool.Pool
	notifyPool   *pgxpool.Pool // Dedicated single-conn pool for LISTEN; nil = use pool
	names        controllerNames
	eventHandler centrifuge.ControlEventHandler
	myNodeID     string
	closeCh      chan struct{}
	closeOnce    sync.Once
	cancelCtx    context.Context
	cancelFunc   context.CancelFunc
	notifyCh     chan struct{}
}

var _ centrifuge.Controller = (*PostgresController)(nil)

func NewPostgresController(node *centrifuge.Node, conf PostgresControllerConfig) (*PostgresController, error) {
	conf.setDefaults()

	ctx := context.Background()

	poolConfig, err := pgxpool.ParseConfig(conf.DSN)
	if err != nil {
		return nil, fmt.Errorf("error parsing postgres DSN: %w", err)
	}
	poolConfig.MaxConns = int32(conf.PoolSize)
	if conf.TLS.Enabled {
		tlsCfg, err := conf.TLS.ToGoTLSConfig("postgres-controller")
		if err != nil {
			return nil, fmt.Errorf("postgres controller: TLS config: %w", err)
		}
		poolConfig.ConnConfig.TLSConfig = tlsCfg
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating postgres pool: %w", err)
	}

	cancelCtx, cancelFunc := context.WithCancel(context.Background())

	c := &PostgresController{
		node:       node,
		conf:       conf,
		pool:       pool,
		names:      newControllerNames(conf.TablePrefix),
		closeCh:    make(chan struct{}),
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
		notifyCh:   make(chan struct{}, 1),
	}

	if conf.NotifyDSN != "" {
		nCfg, err := pgxpool.ParseConfig(conf.NotifyDSN)
		if err != nil {
			pool.Close()
			cancelFunc()
			return nil, fmt.Errorf("error parsing notify DSN: %w", err)
		}
		nCfg.MaxConns = 1
		if conf.TLS.Enabled {
			tlsCfg, err := conf.TLS.ToGoTLSConfig("postgres-controller")
			if err != nil {
				pool.Close()
				cancelFunc()
				return nil, fmt.Errorf("postgres controller: notify TLS config: %w", err)
			}
			nCfg.ConnConfig.TLSConfig = tlsCfg
		}
		nPool, err := pgxpool.NewWithConfig(ctx, nCfg)
		if err != nil {
			pool.Close()
			cancelFunc()
			return nil, fmt.Errorf("error creating notify pool: %w", err)
		}
		c.notifyPool = nPool
	}

	return c, nil
}

func (c *PostgresController) logError(msg string, err error) {
	log.Error().Err(err).Str("controller", "postgres").Msg(msg)
}

// EnsureSchema creates the required database objects idempotently.
func (c *PostgresController) EnsureSchema(ctx context.Context) error {
	// Fast path: version already current and messages table exists.
	var dbVersion int
	err := c.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT schema_version FROM %s WHERE id = 1`, c.names.schemaVersion),
	).Scan(&dbVersion)
	if err == nil && dbVersion == controllerSchemaVersion {
		if _, probeErr := c.pool.Exec(ctx, fmt.Sprintf(
			`SELECT 1 FROM %s LIMIT 0`, c.names.messages)); probeErr == nil {
			// Verify partitioned shape.
			if err := c.verifyPartitionedShape(ctx); err != nil {
				return err
			}
			return c.ensureInitialPartitions(ctx)
		}
	}

	// Render schema template.
	prefix := strings.TrimRight(c.conf.TablePrefix, "_") + "_"
	schemaSQL := strings.NewReplacer("__PREFIX__", prefix).Replace(postgresControllerSchemaTemplate)

	// Split DDL from functions.
	ddl, funcs := splitControllerSchemaSQL(schemaSQL)
	for _, sql := range []string{ddl, funcs} {
		if sql == "" {
			continue
		}
		if err := c.execSchemaWithRetry(ctx, sql); err != nil {
			return err
		}
	}

	// Populate/trim shard_lock rows.
	if _, err := c.pool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s (shard_id) SELECT generate_series(0, $1 - 1) ON CONFLICT DO NOTHING`,
		c.names.shardLock), c.conf.NumShards); err != nil {
		return fmt.Errorf("postgres controller: populate shard_lock: %w", err)
	}
	if _, err := c.pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s WHERE shard_id >= $1`,
		c.names.shardLock), c.conf.NumShards); err != nil {
		return fmt.Errorf("postgres controller: trim shard_lock: %w", err)
	}

	// Verify partitioned shape.
	if err := c.verifyPartitionedShape(ctx); err != nil {
		return err
	}

	// Pre-create partitions.
	if err := c.ensureInitialPartitions(ctx); err != nil {
		return err
	}

	// Update version row.
	if _, err := c.pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s SET schema_version = $1 WHERE id = 1`,
		c.names.schemaVersion), controllerSchemaVersion); err != nil {
		c.logError("schema version update", err)
	}

	return nil
}

// splitControllerSchemaSQL separates DDL from function definitions.
func splitControllerSchemaSQL(sql string) (ddl, funcs string) {
	const marker = "CREATE OR REPLACE FUNCTION"
	i := strings.Index(sql, marker)
	if i < 0 {
		return sql, ""
	}
	return sql[:i], sql[i:]
}

func (c *PostgresController) execSchemaWithRetry(ctx context.Context, sql string) error {
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		_, err := c.pool.Exec(ctx, sql)
		if err == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "XX000") && attempt < maxRetries-1 {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		return fmt.Errorf("postgres controller: schema exec: %w", err)
	}
	return nil
}

func (c *PostgresController) verifyPartitionedShape(ctx context.Context) error {
	var isPartitioned bool
	err := c.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_partitioned_table
			WHERE partrelid = $1::regclass
		)
	`, c.names.messages).Scan(&isPartitioned)
	if err != nil {
		return nil
	}
	if !isPartitioned {
		return fmt.Errorf(
			"postgres controller: %s exists but is not partitioned; "+
				"drop the existing tables manually: DROP TABLE IF EXISTS %s, %s, %s CASCADE",
			c.names.messages, c.names.messages, c.names.shardLock, c.names.schemaVersion,
		)
	}
	return nil
}

func (c *PostgresController) ensureInitialPartitions(ctx context.Context) error {
	p := c.newPartitioner()
	return p.EnsureLookaheadPartitions(ctx)
}

func (c *PostgresController) newPartitioner() *pgoutbox.Partitioner {
	return &pgoutbox.Partitioner{
		Pool:            c.pool,
		ParentTable:     c.names.messages,
		CleanupInterval: c.conf.PartitionCleanupInterval,
		LookaheadDays:   c.conf.PartitionLookaheadDays,
		RetentionDays:   c.conf.PartitionRetentionDays,
		ErrorFn:         c.logError,
	}
}

// RegisterControlEventHandler is called once during node.Run(). It stores
// the event handler and starts the background workers (outbox poller,
// notification listener, partitioner).
func (c *PostgresController) RegisterControlEventHandler(h centrifuge.ControlEventHandler) error {
	c.eventHandler = h
	c.myNodeID = c.node.ID()

	// Start one outbox worker per shard. With NumShards=1 (default),
	// a single worker polls all messages.
	for i := 0; i < c.conf.NumShards; i++ {
		go c.runOutboxWorker(i)
	}

	// Start notification listener if enabled.
	if c.conf.UseNotify {
		go c.runNotificationListener()
	}

	// Start partition maintenance.
	go c.newPartitioner().Run(c.cancelCtx, c.closeCh)

	log.Info().Str("controller", "postgres").Msg("postgres controller running")
	return nil
}

// PublishControl sends a control message. If nodeID is empty, the message
// is broadcast to all nodes. If nodeID is set, only that node will process it.
func (c *PostgresController) PublishControl(data []byte, nodeID, _ string) error {
	_, err := c.pool.Exec(c.cancelCtx,
		fmt.Sprintf(`SELECT %s($1, $2, $3)`, c.names.publishFunc),
		data, nodeID, c.conf.NumShards,
	)
	if err != nil {
		return fmt.Errorf("postgres controller: publish: %w", err)
	}
	return nil
}

// Run blocks until ctx is cancelled, then closes pools.
func (c *PostgresController) Run(ctx context.Context) error {
	<-ctx.Done()
	c.closeOnce.Do(func() {
		close(c.closeCh)
		c.cancelFunc()
		if c.notifyPool != nil {
			c.notifyPool.Close()
		}
		c.pool.Close()
	})
	return ctx.Err()
}

// initCursor bootstraps the cursor from MAX(id) of the messages table.
func (c *PostgresController) initCursor(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var cursor int64
	err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT COALESCE(MAX(id), 0) FROM %s`, c.names.messages)).Scan(&cursor)
	return cursor, err
}

// runOutboxWorker polls the messages table for new control messages
// belonging to the given shard.
func (c *PostgresController) runOutboxWorker(shardIdx int) {
	w := &pgoutbox.Worker{
		Pool:         c.pool,
		ShardIDs:     []int{shardIdx},
		PollInterval: c.conf.PollInterval,
		NotifyCh:     c.notifyCh,
		InitCursor:   c.initCursor,
		ProcessBatch: c.processControlBatch,
		ErrorFn:      c.logError,
	}
	w.Run(c.cancelCtx, c.closeCh)
}

// runNotificationListener listens for NOTIFY on the controller channel.
func (c *PostgresController) runNotificationListener() {
	pool := c.pool
	if c.notifyPool != nil {
		pool = c.notifyPool
	}
	l := &pgoutbox.NotificationListener{
		Pool:     pool,
		Channel:  c.names.notifyChannel,
		NotifyCh: c.notifyCh,
		ErrorFn:  c.logError,
	}
	l.Run(c.cancelCtx, c.closeCh)
}

// processControlBatch reads a batch of control messages from the outbox
// and dispatches them to the event handler.
func (c *PostgresController) processControlBatch(
	ctx context.Context, pool *pgxpool.Pool, cursor int64, shardIDs []int,
) (int, int64, error) {
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT id, node_id, data
		  FROM %s
		 WHERE id > $1 AND shard_id = ANY($3)
		 ORDER BY id
		 LIMIT $2
	`, c.names.messages), cursor, c.conf.BatchSize, shardIDs)
	if err != nil {
		return 0, cursor, fmt.Errorf("postgres controller: outbox query: %w", err)
	}
	defer rows.Close()

	count := 0
	maxID := cursor
	for rows.Next() {
		var id int64
		var nodeID string
		var data []byte
		if err := rows.Scan(&id, &nodeID, &data); err != nil {
			return count, maxID, fmt.Errorf("postgres controller: outbox scan: %w", err)
		}
		if id > maxID {
			maxID = id
		}
		count++

		// Filter targeted messages.
		if nodeID != "" && nodeID != c.myNodeID {
			continue
		}
		if err := c.eventHandler.HandleControl(data); err != nil {
			c.logError("handle control message", err)
		}
	}
	if err := rows.Err(); err != nil {
		return count, maxID, fmt.Errorf("postgres controller: outbox rows: %w", err)
	}

	return count, maxID, nil
}
