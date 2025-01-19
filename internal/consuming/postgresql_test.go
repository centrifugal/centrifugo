//go:build integration

package consuming

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

const (
	testPGDSN = "postgres://test:test@localhost:5432/test"
)

// setupTestTable creates a test table and notification trigger in the PostgreSQL database.
func setupTestTable(ctx context.Context, tableName string, notificationChannel string) error {
	pool, err := pgxpool.New(ctx, testPGDSN)
	if err != nil {
		return fmt.Errorf("failed to create pool: %w", err)
	}
	defer pool.Close()

	// Define the SQL for creating the table
	createTableSQL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	id BIGSERIAL PRIMARY KEY,
	method text NOT NULL,
	payload BYTEA NOT NULL,
	partition INTEGER NOT NULL
);`, tableName)

	_, err = pool.Exec(ctx, createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create test table: %w", err)
	}

	// Define the SQL for creating the notification trigger.
	createTriggerSQL := fmt.Sprintf(`
CREATE OR REPLACE FUNCTION %s()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify('%s', NEW.partition::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER %s
AFTER INSERT ON %s
FOR EACH ROW
EXECUTE FUNCTION %s();`, notificationChannel, notificationChannel, notificationChannel, tableName, notificationChannel)

	_, err = pool.Exec(ctx, createTriggerSQL)
	if err != nil {
		return fmt.Errorf("failed to create notification trigger: %w", err)
	}

	return nil
}

func insertEvent(ctx context.Context, pool *pgxpool.Pool, tableName string, method string, payload []byte, partition int) error {
	_, err := pool.Exec(ctx, "INSERT INTO "+tableName+" (method, payload, partition) VALUES ($1, $2, $3)", method, payload, partition)
	return err
}

func getEventCount(ctx context.Context, tableName string, partition int) (int, error) {
	pool, err := pgxpool.New(ctx, testPGDSN)
	if err != nil {
		return 0, fmt.Errorf("failed to create pool: %w", err)
	}
	defer pool.Close()
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE partition=$1", tableName)
	err = pool.QueryRow(ctx, query, partition).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error querying count: %w", err)
	}
	return count, nil
}

func ensureEventsRemoved(ctx context.Context, t *testing.T, tableName string, partition int) {
	t.Helper()
	for {
		select {
		case <-ctx.Done():
			require.Fail(t, "timeout checking removed events")
		default:
			eventCount, err := getEventCount(ctx, tableName, partition)
			require.NoError(t, err)
			if eventCount != 0 {
				select {
				case <-ctx.Done():
					require.Fail(t, "timeout checking removed events")
				case <-time.After(time.Second):
					continue
				}
			}
			return
		}
	}
}

func TestPostgresConsumer_GreenScenario(t *testing.T) {
	t.Parallel()
	testTableName := "centrifugo_consumer_test_" + strings.Replace(uuid.New().String(), "-", "_", -1)
	testNotificationChannel := "centrifugo_test_channel_" + strings.Replace(uuid.New().String(), "-", "_", -1)
	testMethod := "method"
	testPayload := []byte(`{"key":"value"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := setupTestTable(ctx, testTableName, testNotificationChannel)
	require.NoError(t, err)

	eventReceived := make(chan struct{})
	consumerClosed := make(chan struct{})

	// Setup consumer
	config := PostgresConfig{
		DSN:                          testPGDSN,
		OutboxTableName:              testTableName,
		PartitionSelectLimit:         10,
		NumPartitions:                1,
		PartitionPollInterval:        configtypes.Duration(300 * time.Millisecond),
		PartitionNotificationChannel: testNotificationChannel,
	}
	consumer, err := NewPostgresConsumer("test", &MockDispatcher{
		onDispatch: func(ctx context.Context, method string, data []byte) error {
			require.Equal(t, testMethod, method)
			require.Equal(t, testPayload, data)
			close(eventReceived)
			return nil
		},
	}, config, newCommonMetrics(prometheus.NewRegistry()))
	require.NoError(t, err)

	// Start the consumer
	go func() {
		err := consumer.Run(ctx)
		require.ErrorIs(t, err, context.Canceled)
		close(consumerClosed)
	}()

	partition := 0
	err = insertEvent(ctx, consumer.pool, testTableName, testMethod, testPayload, partition)
	require.NoError(t, err)

	waitCh(t, eventReceived, 30*time.Second, "timeout waiting for event")
	ensureEventsRemoved(ctx, t, testTableName, 0)
	cancel()
	waitCh(t, consumerClosed, 30*time.Second, "timeout waiting for consumer closed")
}

func TestPostgresConsumer_SeveralConsumers(t *testing.T) {
	t.Parallel()
	testTableName := "centrifugo_consumer_test_" + strings.Replace(uuid.New().String(), "-", "_", -1)
	testNotificationChannel := "centrifugo_test_channel_" + strings.Replace(uuid.New().String(), "-", "_", -1)
	testMethod := "method"
	testPayload := []byte(`{"key":"value"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := setupTestTable(ctx, testTableName, testNotificationChannel)
	require.NoError(t, err)

	eventReceived := make(chan struct{})
	consumerClosed := make(chan struct{})

	config := PostgresConfig{
		DSN:                          testPGDSN,
		OutboxTableName:              testTableName,
		PartitionSelectLimit:         10,
		NumPartitions:                1,
		PartitionPollInterval:        configtypes.Duration(300 * time.Millisecond),
		PartitionNotificationChannel: testNotificationChannel,
	}

	var pool *pgxpool.Pool

	numConsumers := 10

	for i := 0; i < numConsumers; i++ {
		consumer, err := NewPostgresConsumer("test", &MockDispatcher{
			onDispatch: func(ctx context.Context, method string, data []byte) error {
				require.Equal(t, testMethod, method)
				require.Equal(t, testPayload, data)
				close(eventReceived)
				return nil
			},
		}, config, newCommonMetrics(prometheus.NewRegistry()))
		require.NoError(t, err)

		pool = consumer.pool

		go func() {
			err := consumer.Run(ctx)
			require.ErrorIs(t, err, context.Canceled)
			consumerClosed <- struct{}{}
		}()
	}

	partition := 0
	err = insertEvent(ctx, pool, testTableName, testMethod, testPayload, partition)
	require.NoError(t, err)

	waitCh(t, eventReceived, 30*time.Second, "timeout waiting for event")
	ensureEventsRemoved(ctx, t, testTableName, partition)
	cancel()
	for i := 0; i < numConsumers; i++ {
		waitCh(t, consumerClosed, 30*time.Second, "timeout waiting for consumer closed")
	}
}

func TestPostgresConsumer_NotificationTrigger(t *testing.T) {
	t.Parallel()
	testTableName := "centrifugo_consumer_test_" + strings.Replace(uuid.New().String(), "-", "_", -1)
	testNotificationChannel := "centrifugo_test_channel_" + strings.Replace(uuid.New().String(), "-", "_", -1)
	testMethod := "method"
	testPayload := []byte(`{"key":"value"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := setupTestTable(ctx, testTableName, testNotificationChannel)
	require.NoError(t, err)

	eventsReceived := make(chan struct{})
	consumerClosed := make(chan struct{})

	config := PostgresConfig{
		DSN:                          testPGDSN,
		OutboxTableName:              testTableName,
		PartitionSelectLimit:         1,
		NumPartitions:                1,
		PartitionPollInterval:        configtypes.Duration(300 * time.Hour), // Set a long poll interval
		PartitionNotificationChannel: testNotificationChannel,
	}

	numEvents := 0

	consumer, err := NewPostgresConsumer("test", &MockDispatcher{
		onDispatch: func(ctx context.Context, method string, data []byte) error {
			require.Equal(t, testMethod, method)
			require.Equal(t, testPayload, data)
			numEvents++
			eventsReceived <- struct{}{}
			return nil
		},
	}, config, newCommonMetrics(prometheus.NewRegistry()))
	require.NoError(t, err)

	go func() {
		err := consumer.Run(ctx)
		require.ErrorIs(t, err, context.Canceled)
		close(consumerClosed)
	}()

	partition := 0

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(200 * time.Millisecond):
				err = insertEvent(ctx, consumer.pool, testTableName, testMethod, testPayload, partition)
				require.NoError(t, err)
			}
		}
	}()

	waitCh(t, eventsReceived, 30*time.Second, "timeout waiting for event 1")
	waitCh(t, eventsReceived, 30*time.Second, "timeout waiting for event 2")
	waitCh(t, eventsReceived, 30*time.Second, "timeout waiting for event 3")

	cancel()
	waitCh(t, consumerClosed, 30*time.Second, "timeout waiting for consumer closed")
}

func TestPostgresConsumer_DifferentPartitions(t *testing.T) {
	t.Parallel()
	testTableName := "centrifugo_consumer_test_" + strings.Replace(uuid.New().String(), "-", "_", -1)
	testNotificationChannel := "centrifugo_test_channel_" + strings.Replace(uuid.New().String(), "-", "_", -1)
	testMethod := "method"
	testPayload := []byte(`{"key":"value"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := setupTestTable(ctx, testTableName, testNotificationChannel)
	require.NoError(t, err)

	eventsReceived := make(chan struct{})
	consumerClosed := make(chan struct{})

	config := PostgresConfig{
		DSN:                          testPGDSN,
		OutboxTableName:              testTableName,
		PartitionSelectLimit:         10,
		NumPartitions:                2,
		PartitionPollInterval:        configtypes.Duration(100 * time.Millisecond),
		PartitionNotificationChannel: testNotificationChannel,
	}

	numEvents := 0

	var dispatchMu sync.Mutex

	consumer, err := NewPostgresConsumer("test", &MockDispatcher{
		onDispatch: func(ctx context.Context, method string, data []byte) error {
			dispatchMu.Lock()
			defer dispatchMu.Unlock()
			require.Equal(t, testMethod, method)
			require.Equal(t, testPayload, data)
			numEvents++
			eventsReceived <- struct{}{}
			return nil
		},
	}, config, newCommonMetrics(prometheus.NewRegistry()))
	require.NoError(t, err)

	go func() {
		err := consumer.Run(ctx)
		require.ErrorIs(t, err, context.Canceled)
		close(consumerClosed)
	}()

	err = insertEvent(ctx, consumer.pool, testTableName, testMethod, testPayload, 0)
	require.NoError(t, err)
	err = insertEvent(ctx, consumer.pool, testTableName, testMethod, testPayload, 1)
	require.NoError(t, err)

	waitCh(t, eventsReceived, 30*time.Second, "timeout waiting for event 1")
	waitCh(t, eventsReceived, 30*time.Second, "timeout waiting for event 2")
	ensureEventsRemoved(ctx, t, testTableName, 0)
	ensureEventsRemoved(ctx, t, testTableName, 1)

	cancel()
	waitCh(t, consumerClosed, 30*time.Second, "timeout waiting for consumer closed")
}
