//go:build integration

package controllers

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

func getPostgresConnString(tb testing.TB) string {
	connString := os.Getenv("CENTRIFUGE_POSTGRES_URL")
	if connString == "" {
		connString = "postgres://test:test@localhost:5432/test?sslmode=disable"
	}
	return connString
}

// testControlEventHandler implements centrifuge.ControlEventHandler for tests.
type testControlEventHandler struct {
	HandleControlFunc func(data []byte) error
}

func (h *testControlEventHandler) HandleControl(data []byte) error {
	if h.HandleControlFunc != nil {
		return h.HandleControlFunc(data)
	}
	return nil
}

// initializedPrefixes tracks table prefixes already dropped+created in this
// test process. Tests that share a prefix between two controllers (e.g.
// broadcast-style tests) must not have the second helper drop tables that
// the first controller's outbox worker is already polling — doing so causes
// transient `relation does not exist` errors and lost messages.
var (
	initializedPrefixesMu sync.Mutex
	initializedPrefixes   = map[string]bool{}
)

// newTestPostgresController creates a controller with a unique table prefix
// for test isolation, runs EnsureSchema, and registers the handler to start
// background workers. Does NOT call node.Run() to avoid the node's internal
// heartbeat/control message traffic polluting test assertions.
func newTestPostgresController(tb testing.TB, conf PostgresControllerConfig, handler *testControlEventHandler) *PostgresController {
	tb.Helper()
	connString := getPostgresConnString(tb)

	if conf.DSN == "" {
		conf.DSN = connString
	}
	if conf.TablePrefix == "" {
		conf.TablePrefix = fmt.Sprintf("test_%d", time.Now().UnixNano()%100000)
	}
	if conf.PollInterval <= 0 {
		conf.PollInterval = 10 * time.Millisecond
	}
	conf.UseNotify = true

	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(tb, err)

	c, err := NewPostgresController(node, conf)
	require.NoError(tb, err)

	ctx := context.Background()

	// Drop schema only the first time we see this prefix in the test process.
	// Subsequent controllers for the same prefix attach to the existing schema
	// via EnsureSchema (idempotent fast-path) so we don't yank tables out from
	// under another controller's running workers.
	initializedPrefixesMu.Lock()
	firstForPrefix := !initializedPrefixes[conf.TablePrefix]
	if firstForPrefix {
		initializedPrefixes[conf.TablePrefix] = true
	}
	initializedPrefixesMu.Unlock()
	if firstForPrefix {
		dropTestControllerSchema(tb, c)
	}
	require.NoError(tb, c.EnsureSchema(ctx))
	cleanupTestControllerMessages(ctx, c)

	// Register the handler directly (starts outbox worker, listener, partitioner).
	if handler != nil {
		require.NoError(tb, c.RegisterControlEventHandler(handler))
	}

	tb.Cleanup(func() {
		c.closeOnce.Do(func() {
			close(c.closeCh)
			c.cancelFunc()
			c.pool.Close()
		})
		_ = node.Shutdown(context.Background())
	})

	return c
}

func dropTestControllerSchema(tb testing.TB, c *PostgresController) {
	tb.Helper()
	ctx := context.Background()
	for _, tbl := range []string{c.names.messages, c.names.shardLock, c.names.schemaVersion} {
		_, _ = c.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", tbl))
	}
	_, _ = c.pool.Exec(ctx, fmt.Sprintf("DROP FUNCTION IF EXISTS %s", c.names.publishFunc))
}

func cleanupTestControllerMessages(ctx context.Context, c *PostgresController) {
	rows, err := c.pool.Query(ctx, `
		SELECT c.relname
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class pp ON pp.oid = i.inhparent
		WHERE pp.relname = $1
	`, c.names.messages)
	if err != nil {
		return
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			parts = append(parts, name)
		}
	}
	rows.Close()
	for _, part := range parts {
		_, _ = c.pool.Exec(ctx, fmt.Sprintf("TRUNCATE %s", part))
	}
}

func waitForMessages(t *testing.T, counter *atomic.Int64, expectedCount int64, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if counter.Load() >= expectedCount {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timeout: expected %d messages, got %d", expectedCount, counter.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestPostgresController_PublishAndReceive(t *testing.T) {
	var received atomic.Int64
	var receivedData []byte
	var mu sync.Mutex
	doneCh := make(chan struct{})

	handler := &testControlEventHandler{
		HandleControlFunc: func(data []byte) error {
			mu.Lock()
			receivedData = append([]byte{}, data...)
			mu.Unlock()
			if received.Add(1) == 1 {
				close(doneCh)
			}
			return nil
		},
	}

	c := newTestPostgresController(t, PostgresControllerConfig{}, handler)

	time.Sleep(50 * time.Millisecond)

	testData := []byte("hello-control-message")
	require.NoError(t, c.PublishControl(testData, "", ""))

	select {
	case <-doneCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for control message")
	}

	mu.Lock()
	require.Equal(t, testData, receivedData)
	mu.Unlock()
}

func TestPostgresController_BroadcastReachesAll(t *testing.T) {
	connString := getPostgresConnString(t)
	prefix := fmt.Sprintf("test_%d", time.Now().UnixNano()%100000)

	var count1, count2 atomic.Int64
	done1 := make(chan struct{})
	done2 := make(chan struct{})

	h1 := &testControlEventHandler{
		HandleControlFunc: func(data []byte) error {
			if count1.Add(1) == 1 {
				close(done1)
			}
			return nil
		},
	}
	h2 := &testControlEventHandler{
		HandleControlFunc: func(data []byte) error {
			if count2.Add(1) == 1 {
				close(done2)
			}
			return nil
		},
	}

	c1 := newTestPostgresController(t, PostgresControllerConfig{
		DSN:         connString,
		TablePrefix: prefix,
	}, h1)
	_ = newTestPostgresController(t, PostgresControllerConfig{
		DSN:            connString,
		TablePrefix:    prefix,
	}, h2)

	time.Sleep(50 * time.Millisecond)

	require.NoError(t, c1.PublishControl([]byte("broadcast"), "", ""))

	select {
	case <-done1:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: c1 didn't receive broadcast")
	}
	select {
	case <-done2:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: c2 didn't receive broadcast")
	}
}

func TestPostgresController_TargetedMessage(t *testing.T) {
	connString := getPostgresConnString(t)
	prefix := fmt.Sprintf("test_%d", time.Now().UnixNano()%100000)

	var count1, count2 atomic.Int64

	h1 := &testControlEventHandler{
		HandleControlFunc: func(data []byte) error {
			count1.Add(1)
			return nil
		},
	}
	h2 := &testControlEventHandler{
		HandleControlFunc: func(data []byte) error {
			count2.Add(1)
			return nil
		},
	}

	c1 := newTestPostgresController(t, PostgresControllerConfig{
		DSN:         connString,
		TablePrefix: prefix,
	}, h1)
	c2 := newTestPostgresController(t, PostgresControllerConfig{
		DSN:            connString,
		TablePrefix:    prefix,
	}, h2)

	time.Sleep(50 * time.Millisecond)

	// Send targeted message to c2 only.
	require.NoError(t, c1.PublishControl([]byte("targeted"), c2.myNodeID, ""))

	waitForMessages(t, &count2, 1, 5*time.Second)

	// Give c1 time to NOT receive it.
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, int64(0), count1.Load(), "c1 should not receive targeted message for c2")
	require.Equal(t, int64(1), count2.Load())
}

func TestPostgresController_UseNotify_LowLatency(t *testing.T) {
	receivedCh := make(chan time.Time, 1)
	handler := &testControlEventHandler{
		HandleControlFunc: func(data []byte) error {
			select {
			case receivedCh <- time.Now():
			default:
			}
			return nil
		},
	}

	c := newTestPostgresController(t, PostgresControllerConfig{
		PollInterval: 500 * time.Millisecond,
		UseNotify:    true,
	}, handler)

	time.Sleep(100 * time.Millisecond) // Let listener establish LISTEN.

	start := time.Now()
	require.NoError(t, c.PublishControl([]byte("fast"), "", ""))

	select {
	case received := <-receivedCh:
		latency := received.Sub(start)
		t.Logf("NOTIFY latency: %v", latency)
		require.Less(t, latency, 200*time.Millisecond, "NOTIFY should deliver well under PollInterval")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for NOTIFY-driven delivery")
	}
}

func TestPostgresController_SchemaCreation(t *testing.T) {
	connString := getPostgresConnString(t)
	prefix := fmt.Sprintf("test_%d", time.Now().UnixNano()%100000)

	c := newTestPostgresController(t, PostgresControllerConfig{
		DSN:         connString,
		TablePrefix: prefix,
	}, nil)

	ctx := context.Background()
	names := c.names

	// Verify messages table exists and is partitioned.
	var isPartitioned bool
	err := c.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_partitioned_table
			WHERE partrelid = $1::regclass
		)
	`, names.messages).Scan(&isPartitioned)
	require.NoError(t, err)
	require.True(t, isPartitioned, "messages table should be partitioned")

	// Verify schema_version table.
	var version int
	err = c.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT schema_version FROM %s WHERE id = 1`, names.schemaVersion)).Scan(&version)
	require.NoError(t, err)
	require.Equal(t, controllerSchemaVersion, version)

	// Verify shard_lock table exists with correct row count.
	var shardCount int
	err = c.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT count(*) FROM %s`, names.shardLock)).Scan(&shardCount)
	require.NoError(t, err)
	require.Equal(t, 1, shardCount, "shard_lock should have 1 row (NumShards=1)")

	// Verify publish function exists.
	var funcExists bool
	err = c.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM pg_proc WHERE proname = $1)
	`, names.publishFunc).Scan(&funcExists)
	require.NoError(t, err)
	require.True(t, funcExists, "publish function should exist")

	// Verify partitions were pre-created.
	var partCount int
	err = c.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class pp ON pp.oid = i.inhparent
		WHERE pp.relname = $1
	`, names.messages).Scan(&partCount)
	require.NoError(t, err)
	require.GreaterOrEqual(t, partCount, 1, "at least today's partition should exist")
}

func TestPostgresController_TablePrefix(t *testing.T) {
	connString := getPostgresConnString(t)
	prefix := fmt.Sprintf("custom_%d", time.Now().UnixNano()%100000)

	var received atomic.Int64
	doneCh := make(chan struct{})
	handler := &testControlEventHandler{
		HandleControlFunc: func(data []byte) error {
			if received.Add(1) == 1 {
				close(doneCh)
			}
			return nil
		},
	}

	c := newTestPostgresController(t, PostgresControllerConfig{
		DSN:         connString,
		TablePrefix: prefix,
	}, handler)

	// Verify the names use the custom prefix.
	expectedPrefix := prefix + "_"
	require.Contains(t, c.names.messages, expectedPrefix+"controller_messages")
	require.Contains(t, c.names.publishFunc, expectedPrefix+"controller_publish")
	require.Contains(t, c.names.notifyChannel, expectedPrefix+"controller_notify")

	time.Sleep(50 * time.Millisecond)

	require.NoError(t, c.PublishControl([]byte("prefix-test"), "", ""))

	select {
	case <-doneCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: custom prefix publish/receive failed")
	}
}

func TestPostgresController_HighVolume(t *testing.T) {
	const msgCount = 1000
	var received atomic.Int64
	doneCh := make(chan struct{})
	handler := &testControlEventHandler{
		HandleControlFunc: func(data []byte) error {
			if received.Add(1) == int64(msgCount) {
				close(doneCh)
			}
			return nil
		},
	}

	c := newTestPostgresController(t, PostgresControllerConfig{}, handler)

	time.Sleep(50 * time.Millisecond)

	for i := 0; i < msgCount; i++ {
		require.NoError(t, c.PublishControl([]byte(fmt.Sprintf("msg-%d", i)), "", ""))
	}

	select {
	case <-doneCh:
		t.Logf("all %d messages received", msgCount)
	case <-time.After(30 * time.Second):
		t.Fatalf("timeout: received %d/%d messages", received.Load(), msgCount)
	}
}

func TestPostgresController_ReconnectResumesFromCursor(t *testing.T) {
	connString := getPostgresConnString(t)
	prefix := fmt.Sprintf("test_%d", time.Now().UnixNano()%100000)

	var count1 atomic.Int64
	h1 := &testControlEventHandler{
		HandleControlFunc: func(data []byte) error {
			count1.Add(1)
			return nil
		},
	}

	c1 := newTestPostgresController(t, PostgresControllerConfig{
		DSN:         connString,
		TablePrefix: prefix,
	}, h1)

	time.Sleep(50 * time.Millisecond)

	// Publish messages that c1 will consume.
	for i := 0; i < 5; i++ {
		require.NoError(t, c1.PublishControl([]byte(fmt.Sprintf("old-%d", i)), "", ""))
	}
	waitForMessages(t, &count1, 5, 5*time.Second)

	// Create a second controller with the same prefix — should init cursor
	// from MAX(id) and NOT re-deliver old messages.
	var count2 atomic.Int64
	h2 := &testControlEventHandler{
		HandleControlFunc: func(data []byte) error {
			count2.Add(1)
			return nil
		},
	}

	_ = newTestPostgresController(t, PostgresControllerConfig{
		DSN:            connString,
		TablePrefix:    prefix,
	}, h2)

	time.Sleep(200 * time.Millisecond)

	require.Equal(t, int64(0), count2.Load(), "new controller should not re-deliver old messages")

	// Publish a new message — c2 should get it.
	require.NoError(t, c1.PublishControl([]byte("new-msg"), "", ""))
	waitForMessages(t, &count2, 1, 5*time.Second)
}

func TestPostgresController_EnsureSchema_Idempotent(t *testing.T) {
	connString := getPostgresConnString(t)
	prefix := fmt.Sprintf("test_%d", time.Now().UnixNano()%100000)

	c := newTestPostgresController(t, PostgresControllerConfig{
		DSN:         connString,
		TablePrefix: prefix,
	}, nil)

	ctx := context.Background()

	require.NoError(t, c.EnsureSchema(ctx))
	require.NoError(t, c.EnsureSchema(ctx))

	var version int
	err := c.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT schema_version FROM %s WHERE id = 1`, c.names.schemaVersion)).Scan(&version)
	require.NoError(t, err)
	require.Equal(t, controllerSchemaVersion, version)
}

func TestPostgresController_PartitionRetention(t *testing.T) {
	connString := getPostgresConnString(t)
	prefix := fmt.Sprintf("test_%d", time.Now().UnixNano()%100000)

	c := newTestPostgresController(t, PostgresControllerConfig{
		DSN:                    connString,
		TablePrefix:            prefix,
		PartitionRetentionDays: 1,
		PartitionLookaheadDays: 2,
	}, nil)

	ctx := context.Background()

	// Create an old partition manually (7 days ago).
	oldDate := time.Now().UTC().AddDate(0, 0, -7)
	nextDay := oldDate.AddDate(0, 0, 1)
	oldPartName := fmt.Sprintf("%s_%s", c.names.messages, oldDate.Format("2006_01_02"))
	_, err := c.pool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
		oldPartName, c.names.messages,
		oldDate.Format("2006-01-02"), nextDay.Format("2006-01-02"),
	))
	require.NoError(t, err)

	var exists bool
	err = c.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname = $1)`, oldPartName).Scan(&exists)
	require.NoError(t, err)
	require.True(t, exists, "old partition should exist before cleanup")

	p := c.newPartitioner()
	p.DropOldPartitions(ctx)

	err = c.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname = $1)`, oldPartName).Scan(&exists)
	require.NoError(t, err)
	require.False(t, exists, "old partition should be dropped by retention cleanup")
}

func TestPostgresController_ConcurrentPublish(t *testing.T) {
	// With shard_lock serialization, concurrent publishes are serialized
	// within each shard — no BIGSERIAL gaps, 100% delivery guaranteed.
	const goroutines = 10
	const msgsPerGoroutine = 50
	totalMsgs := int64(goroutines * msgsPerGoroutine)

	var received atomic.Int64
	doneCh := make(chan struct{})
	handler := &testControlEventHandler{
		HandleControlFunc: func(data []byte) error {
			if received.Add(1) == totalMsgs {
				close(doneCh)
			}
			return nil
		},
	}

	c := newTestPostgresController(t, PostgresControllerConfig{}, handler)

	time.Sleep(50 * time.Millisecond)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < msgsPerGoroutine; i++ {
				err := c.PublishControl([]byte(fmt.Sprintf("g%d-m%d", gid, i)), "", "")
				require.NoError(t, err)
			}
		}(g)
	}
	wg.Wait()

	select {
	case <-doneCh:
		t.Logf("all %d concurrent messages received", totalMsgs)
	case <-time.After(30 * time.Second):
		t.Fatalf("timeout: received %d/%d concurrent messages", received.Load(), totalMsgs)
	}
}

func TestPostgresController_DirectPoolPublish(t *testing.T) {
	var received atomic.Int64
	doneCh := make(chan struct{})
	handler := &testControlEventHandler{
		HandleControlFunc: func(data []byte) error {
			if received.Add(1) == 1 {
				close(doneCh)
			}
			return nil
		},
	}

	c := newTestPostgresController(t, PostgresControllerConfig{}, handler)

	time.Sleep(50 * time.Millisecond)

	// Direct INSERT + NOTIFY (simulating what the SQL function does).
	ctx := context.Background()
	_, err := c.pool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s (node_id, data) VALUES ('', $1)`, c.names.messages),
		[]byte("direct-insert"),
	)
	require.NoError(t, err)
	_, err = c.pool.Exec(ctx, fmt.Sprintf("NOTIFY %s", c.names.notifyChannel))
	require.NoError(t, err)

	select {
	case <-doneCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: direct insert not picked up by outbox worker")
	}
}

func TestPostgresController_PoolSizeDefaults(t *testing.T) {
	var conf PostgresControllerConfig
	conf.setDefaults()

	require.Equal(t, 8, conf.PoolSize)
	require.Equal(t, 1, conf.NumShards)
	require.Equal(t, "cf", conf.TablePrefix)
	require.Equal(t, 50*time.Millisecond, conf.PollInterval)
	require.Equal(t, 2, conf.PartitionLookaheadDays)
	require.Equal(t, 1, conf.PartitionRetentionDays)
	require.Equal(t, time.Minute, conf.PartitionCleanupInterval)
	require.Equal(t, 1000, conf.BatchSize)
}

func TestPostgresController_NewControllerNames(t *testing.T) {
	names := newControllerNames("cf")
	require.Equal(t, "cf_controller_messages", names.messages)
	require.Equal(t, "cf_controller_shard_lock", names.shardLock)
	require.Equal(t, "cf_controller_schema_version", names.schemaVersion)
	require.Equal(t, "cf_controller_publish", names.publishFunc)
	require.Equal(t, "cf_controller_notify", names.notifyChannel)

	// Trailing underscore is trimmed.
	names2 := newControllerNames("cf_")
	require.Equal(t, names, names2)

	// Custom prefix.
	names3 := newControllerNames("prod_us")
	require.Equal(t, "prod_us_controller_messages", names3.messages)
	require.Equal(t, "prod_us_controller_shard_lock", names3.shardLock)

	// Bare prefix without trailing underscore.
	names4 := newControllerNames("myapp")
	require.Equal(t, "myapp_controller_messages", names4.messages)
}

func TestPostgresController_VerifyInterface(t *testing.T) {
	// Compile-time check that PostgresController implements centrifuge.Controller.
	var _ centrifuge.Controller = (*PostgresController)(nil)

	// Also verify it doesn't need the node to have a broker set.
	connString := getPostgresConnString(t)
	node, _ := centrifuge.New(centrifuge.Config{})
	c, err := NewPostgresController(node, PostgresControllerConfig{
		DSN:         connString,
		TablePrefix: fmt.Sprintf("test_%d", time.Now().UnixNano()%100000),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		c.pool.Close()
	})
	require.NotNil(t, c)
}

func TestPostgresController_PublishControlWithPool(t *testing.T) {
	// Test that PublishControl works independently (no outbox worker needed).
	connString := getPostgresConnString(t)
	prefix := fmt.Sprintf("test_%d", time.Now().UnixNano()%100000)

	node, _ := centrifuge.New(centrifuge.Config{})
	c, err := NewPostgresController(node, PostgresControllerConfig{
		DSN:         connString,
		TablePrefix: prefix,
	})
	require.NoError(t, err)

	ctx := context.Background()
	dropTestControllerSchema(t, c)
	require.NoError(t, c.EnsureSchema(ctx))

	t.Cleanup(func() {
		c.pool.Close()
	})

	// Publish without starting workers — the row should be inserted.
	require.NoError(t, c.PublishControl([]byte("write-only"), "", ""))
	require.NoError(t, c.PublishControl([]byte("targeted"), "some-node", ""))

	// Verify rows in the table.
	var count int
	err = c.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT count(*) FROM %s`, c.names.messages)).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	// Verify the targeted message has the right node_id.
	var nodeID string
	err = c.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT node_id FROM %s ORDER BY id DESC LIMIT 1`, c.names.messages)).Scan(&nodeID)
	require.NoError(t, err)
	require.Equal(t, "some-node", nodeID)
}

