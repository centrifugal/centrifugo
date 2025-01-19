package consuming

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

const (
	defaultNumPartitions        = 1
	defaultPartitionSelectLimit = 100
)

func NewPostgresConsumer(
	name string, dispatcher Dispatcher, config PostgresConfig, metrics *commonMetrics,
) (*PostgresConsumer, error) {
	if config.DSN == "" {
		return nil, errors.New("dsn is required")
	}
	if config.OutboxTableName == "" {
		return nil, errors.New("outbox_table_name is required")
	}
	if config.NumPartitions == 0 {
		config.NumPartitions = defaultNumPartitions
	}
	if config.PartitionSelectLimit == 0 {
		config.PartitionSelectLimit = defaultPartitionSelectLimit
	}
	if config.PartitionPollInterval == 0 {
		config.PartitionPollInterval = configtypes.Duration(300 * time.Millisecond)
	}
	conf, err := pgxpool.ParseConfig(config.DSN)
	if err != nil {
		return nil, fmt.Errorf("error parsing postgresql DSN: %w", err)
	}
	if config.TLS.Enabled {
		tlsConfig, err := config.TLS.ToGoTLSConfig("postgresql:" + name)
		if err != nil {
			return nil, fmt.Errorf("error creating postgresql TLS config: %w", err)
		}
		conf.ConnConfig.TLSConfig = tlsConfig
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), conf)
	if err != nil {
		return nil, fmt.Errorf("error creating postgresql pool: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = pool.Ping(ctx)
	if err != nil {
		return nil, err
	}
	return &PostgresConsumer{
		name:       name,
		pool:       pool,
		dispatcher: dispatcher,
		config:     config,
		lockPrefix: "centrifugo_partition_lock_" + name,
		metrics:    metrics,
	}, nil
}

type PostgresConfig = configtypes.PostgresConsumerConfig

type PostgresConsumer struct {
	name       string
	pool       *pgxpool.Pool
	config     PostgresConfig
	dispatcher Dispatcher
	lockPrefix string
	metrics    *commonMetrics
}

type PostgresEvent struct {
	ID        int64
	Method    string
	Payload   []byte
	Partition int64
}

func (c *PostgresConsumer) listenForNotifications(ctx context.Context, triggerChannels []chan struct{}) error {
	conn, err := c.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("error acquiring connection: %w", err)
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, "LISTEN "+c.config.PartitionNotificationChannel)
	if err != nil {
		return fmt.Errorf("error executing LISTEN command: %w", err)
	}

	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("error waiting postgresql notifications: %w", err)
			}
		}
		partition, err := strconv.Atoi(notification.Payload)
		if err != nil {
			log.Error().Err(err).Str("consumer_name", c.name).Msg("error converting postgresql notification")
			continue
		}

		if partition > len(triggerChannels)-1 {
			log.Error().Str("consumer_name", c.name).Int("partition", partition).Msg("outbox partition is larger than configured number")
			continue
		}
		select {
		case triggerChannels[partition] <- struct{}{}:
		default:
		}
	}
}

func (c *PostgresConsumer) processOnce(ctx context.Context, partition int) (int, error) {
	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return 0, fmt.Errorf("error starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Acquire an advisory lock for partition. This allows us to process all the rows
	// from partition in order.
	lockName := c.lockPrefix + strconv.Itoa(partition)
	_, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", lockName)
	if err != nil {
		return 0, fmt.Errorf("unable to acquire advisory lock: %w", err)
	}

	sql := `
	SELECT
		id, method, payload, partition
	FROM %s
	WHERE partition=$1
	ORDER BY id ASC
	LIMIT $2`

	rows, err := tx.Query(ctx, fmt.Sprintf(sql, c.config.OutboxTableName), partition, c.config.PartitionSelectLimit)
	if err != nil {
		return 0, fmt.Errorf("error selecting outbox events: %w", err)
	}
	defer rows.Close()

	var events []PostgresEvent

	numProcessedRows := 0
	for rows.Next() {
		var event PostgresEvent
		err := rows.Scan(&event.ID, &event.Method, &event.Payload, &event.Partition)
		if err != nil {
			return 0, fmt.Errorf("error scanning event: %w", err)
		}
		events = append(events, event)
	}

	err = rows.Err()
	if err != nil {
		return 0, fmt.Errorf("rows error: %w", err)
	}

	idsToDelete := make([]int64, 0, len(events))

	var dispatchErr error

	for _, event := range events {
		dispatchErr = c.dispatcher.Dispatch(context.Background(), event.Method, event.Payload)
		if dispatchErr != nil {
			// Stop here, all processed events will be removed, and we will start from this one.
			log.Error().Err(dispatchErr).Str("consumer_name", c.name).Str("method", event.Method).Msg("error processing consumed event")
			break
		} else {
			numProcessedRows++
			idsToDelete = append(idsToDelete, event.ID)
		}
	}

	// Delete processed events.
	if len(idsToDelete) > 0 {
		_, err = tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = ANY($1)", c.config.OutboxTableName), idsToDelete)
		if err != nil {
			return 0, fmt.Errorf("error deleting outbox events: %w", err)
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return 0, fmt.Errorf("error commiting transaction: %w", err)
	}

	return numProcessedRows, dispatchErr
}

func (c *PostgresConsumer) Run(ctx context.Context) error {
	defer c.pool.Close()

	eg, ctx := errgroup.WithContext(ctx)

	// Process outbox faster using LISTEN/NOTIFY.
	partitionTriggerChannels := make([]chan struct{}, c.config.NumPartitions)
	for i := 0; i < c.config.NumPartitions; i++ {
		partitionTriggerChannels[i] = make(chan struct{}, 1)
	}

	if c.config.PartitionNotificationChannel != "" {
		eg.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				err := c.listenForNotifications(ctx, partitionTriggerChannels)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return ctx.Err()
					}
					log.Error().Err(err).Str("consumer_name", c.name).Msg("error listening outbox notifications")
					select {
					case <-time.After(time.Second):
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
		})
	}

	pollInterval := c.config.PartitionPollInterval

	for i := 0; i < c.config.NumPartitions; i++ {
		i := i
		eg.Go(func() error {
			var backoffDuration time.Duration = 0
			retries := 0
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				numRows, err := c.processOnce(ctx, i)
				c.metrics.processedTotal.WithLabelValues(c.name).Add(float64(numRows))
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return err
					}
					retries++
					backoffDuration = getNextBackoffDuration(backoffDuration, retries)
					c.metrics.errorsTotal.WithLabelValues(c.name).Inc()
					log.Error().Err(err).Str("consumer_name", c.name).Int("partition", i).Msg("error processing postgresql outbox")
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(backoffDuration):
						continue
					}
				}
				retries = 0
				backoffDuration = 0
				if numRows < c.config.PartitionSelectLimit {
					// Sleep until poll interval or notification for events in partition.
					// If worker processed rows equal to the limit then we don't want to sleep
					// here as there could be more events in the table potentially.
					// If worker processed less than a limit events - then it means table
					// is empty now or some events
					select {
					case <-time.After(pollInterval.ToDuration()):
					case <-partitionTriggerChannels[i]:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
		})
	}

	return eg.Wait()
}
