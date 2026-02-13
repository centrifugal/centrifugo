package consuming

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/api"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/logging"
	"github.com/centrifugal/centrifugo/v6/internal/metrics"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"github.com/twmb/franz-go/pkg/sasl/aws"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

type KafkaConfig = configtypes.KafkaConsumerConfig

type testOnlyConfig struct {
	topicPartitionBeforePauseCh    chan topicPartition
	topicPartitionPauseProceedCh   chan struct{}
	fetchTopicPartitionSubmittedCh chan kgo.FetchTopicPartition
	// leaveGroupOnShutdown when true sends a manual LeaveGroup request on shutdown.
	// Used only in tests to simulate the old behavior with dynamic instance IDs.
	leaveGroupOnShutdown bool
}

type topicPartition struct {
	topic     string
	partition int32
}

type KafkaConsumer struct {
	name           string
	client         *kgo.Client
	dispatcher     Dispatcher
	config         KafkaConfig
	// mu protects the consumers map. During normal operation, concurrent access is already
	// prevented: franz-go serializes assigned/revoked/lost callbacks, and BlockRebalanceOnPoll
	// prevents callbacks from firing while pollUntilFatal reads the map. The mutex is needed
	// for shutdown, when the main goroutine accesses the map (via allTopicPartitions/killConsumers)
	// while the group management goroutine may still be running callbacks. Must not be held
	// during killConsumers' wg.Wait() to avoid blocking concurrent callbacks.
	mu        sync.Mutex
	consumers map[topicPartition]*partitionConsumer
	doneCh         chan struct{}
	common         *consumerCommon
	testOnlyConfig testOnlyConfig
}

func NewKafkaConsumer(
	config KafkaConfig, dispatcher Dispatcher, common *consumerCommon,
) (*KafkaConsumer, error) {
	if len(config.Brokers) == 0 {
		return nil, errors.New("brokers required")
	}
	if len(config.Topics) == 0 {
		return nil, errors.New("topics required")
	}
	if len(config.ConsumerGroup) == 0 {
		return nil, errors.New("consumer_group required")
	}
	if config.MaxPollRecords == 0 {
		config.MaxPollRecords = 100
	}
	if config.FetchMaxWait == 0 {
		config.FetchMaxWait = configtypes.Duration(500 * time.Millisecond)
	}
	if config.PartitionQueueMaxSize == -1 {
		config.PartitionQueueMaxSize = 0
	} else if config.PartitionQueueMaxSize == 0 {
		config.PartitionQueueMaxSize = 1000
	}
	consumer := &KafkaConsumer{
		name:       common.name,
		dispatcher: dispatcher,
		config:     config,
		consumers:  make(map[topicPartition]*partitionConsumer),
		doneCh:     make(chan struct{}),
		common:     common,
	}
	cl, err := consumer.initClient()
	if err != nil {
		return nil, fmt.Errorf("error init Kafka client: %w", err)
	}
	consumer.client = cl
	return consumer, nil
}

const kafkaClientID = "centrifugo"

// leaveGroup sends a manual LeaveGroup request for static members. franz-go's Close does not
// send LeaveGroup for static members by design. This method is only used in tests to simulate
// the old behavior where dynamic instance IDs were combined with a manual LeaveGroup.
func (c *KafkaConsumer) leaveGroup(ctx context.Context, client *kgo.Client) error {
	req := kmsg.NewPtrLeaveGroupRequest()
	req.Group = c.config.ConsumerGroup
	instanceID := c.config.InstanceID
	reason := "shutdown"
	req.Members = []kmsg.LeaveGroupRequestMember{
		{
			InstanceID: &instanceID,
			Reason:     &reason,
		},
	}
	_, err := req.RequestWith(ctx, client)
	return err
}

func (c *KafkaConsumer) initClient() (*kgo.Client, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(c.config.Brokers...),
		kgo.ConsumeTopics(c.config.Topics...),
		kgo.ConsumerGroup(c.config.ConsumerGroup),
		kgo.OnPartitionsAssigned(c.assigned),
		kgo.OnPartitionsRevoked(c.revoked),
		kgo.OnPartitionsLost(c.lost),
		kgo.AutoCommitMarks(),
		kgo.BlockRebalanceOnPoll(),
		kgo.ClientID(kafkaClientID),
		kgo.FetchMaxWait(time.Duration(c.config.FetchMaxWait)),
	}
	if c.config.InstanceID != "" {
		opts = append(opts, kgo.InstanceID(c.config.InstanceID))
	}
	if c.config.FetchMaxBytes > 0 {
		opts = append(opts, kgo.FetchMaxBytes(c.config.FetchMaxBytes))
	}
	if !c.config.FetchReadUncommitted {
		opts = append(opts, kgo.FetchIsolationLevel(kgo.ReadCommitted()))
	} else {
		opts = append(opts, kgo.FetchIsolationLevel(kgo.ReadUncommitted()))
	}
	if c.config.TLS.Enabled {
		tlsConfig, err := c.config.TLS.ToGoTLSConfig("kafka:" + c.name)
		if err != nil {
			return nil, fmt.Errorf("error making TLS configuration: %w", err)
		}
		dialer := &tls.Dialer{
			NetDialer: &net.Dialer{Timeout: 10 * time.Second},
			Config:    tlsConfig,
		}
		opts = append(opts, kgo.Dialer(dialer.DialContext))
	}

	if c.config.SASLMechanism != "" {
		switch c.config.SASLMechanism {
		case "plain":
			opts = append(opts, kgo.SASL(plain.Auth{
				User: c.config.SASLUser,
				Pass: c.config.SASLPassword,
			}.AsMechanism()))
		case "scram-sha-256":
			opts = append(opts, kgo.SASL(scram.Auth{
				User: c.config.SASLUser,
				Pass: c.config.SASLPassword,
			}.AsSha256Mechanism()))
		case "scram-sha-512":
			opts = append(opts, kgo.SASL(scram.Auth{
				User: c.config.SASLUser,
				Pass: c.config.SASLPassword,
			}.AsSha512Mechanism()))
		case "aws-msk-iam":
			opts = append(opts, kgo.SASL(aws.Auth{
				AccessKey: c.config.SASLUser,
				SecretKey: c.config.SASLPassword,
			}.AsManagedStreamingIAMMechanism()))
		default:
			return nil, fmt.Errorf("unsupported SASL mechanism: %s", c.config.SASLMechanism)
		}
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("error initializing client: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("error ping Kafka: %w", err)
	}
	return client, nil
}

func (c *KafkaConsumer) Run(ctx context.Context) error {
	defer func() {
		close(c.doneCh)
		if c.client != nil {
			// Stop all partition consumers first to ensure no more offsets are being marked.
			c.killConsumers(c.allTopicPartitions())
			closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			// Now safe to commit — all partition consumers have stopped marking offsets.
			if err := c.client.CommitMarkedOffsets(closeCtx); err != nil {
				c.common.log.Error().Err(err).Msg("error committing marked offsets on shutdown")
			}
			if c.testOnlyConfig.leaveGroupOnShutdown && c.config.InstanceID != "" {
				if err := c.leaveGroup(closeCtx, c.client); err != nil {
					c.common.log.Error().Err(err).Msg("error leaving consumer group")
				}
			}
			// CloseAllowingRebalance calls AllowRebalance (required since we use
			// BlockRebalanceOnPoll) and then Close. For non-static members (no InstanceID),
			// Close sends LeaveGroup automatically. For static members (InstanceID is set),
			// Close does not send LeaveGroup – so the replacement consumer with the same
			// instance ID can take over partitions seamlessly without triggering a rebalance.
			c.client.CloseAllowingRebalance()
		}
	}()
	for {
		err := c.pollUntilFatal(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return ctx.Err()
			}
			c.common.log.Error().Err(err).Msg("error polling Kafka")
		}
		// Upon returning from polling loop we are re-initializing consumer client.
		c.client.CloseAllowingRebalance()
		c.client = nil
		c.common.log.Info().Msg("re-initializing Kafka consumer client")
		err = c.reInitClient(ctx)
		if err != nil {
			// Only context.Canceled may be returned.
			return err
		}
		c.common.log.Info().Msg("Kafka consumer client re-initialized")
	}
}

func (c *KafkaConsumer) pollUntilFatal(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			fetches := c.client.PollRecords(ctx, c.config.MaxPollRecords)
			if fetches.IsClientClosed() {
				return nil
			}
			fetchErrors := fetches.Errors()
			if len(fetchErrors) > 0 {
				// Non-retryable errors returned. We will restart consumer client, but log errors first.
				var errs []error
				for _, fetchErr := range fetchErrors {
					if errors.Is(fetchErr.Err, context.Canceled) {
						return ctx.Err()
					}
					errs = append(errs, fetchErr.Err)
					c.common.log.Error().Err(fetchErr.Err).Str("topic", fetchErr.Topic).Int32("partition", fetchErr.Partition).Msg("error while polling Kafka")
				}
				return fmt.Errorf("poll error: %w", errors.Join(errs...))
			}

			// Track which partitions we've paused in this poll cycle.
			pausedTopicPartitions := map[topicPartition]struct{}{}

			fetches.EachPartition(func(p kgo.FetchTopicPartition) {
				if len(p.Records) == 0 {
					return
				}

				tp := topicPartition{p.Topic, p.Partition}

				consumer := c.consumers[tp]
				if consumer == nil {
					return
				}

				select {
				case <-ctx.Done():
					return
				case <-consumer.partitionCtx.Done():
					return
				default:
				}

				partitionsToPause := map[string][]int32{p.Topic: {p.Partition}}
				if _, paused := pausedTopicPartitions[tp]; !paused {
					// Only pause if queue threshold would be exceeded after adding records, or if threshold is 0 (always pause).
					shouldPause := consumer.queue.NumRecords()+len(p.Records) >= c.config.PartitionQueueMaxSize
					if shouldPause {
						// Pause partition BEFORE submitting to queue to avoid race condition
						// between pause and resume operations.
						if c.testOnlyConfig.topicPartitionBeforePauseCh != nil {
							c.testOnlyConfig.topicPartitionBeforePauseCh <- tp
						}
						if c.testOnlyConfig.topicPartitionPauseProceedCh != nil {
							<-c.testOnlyConfig.topicPartitionPauseProceedCh
						}
						c.client.PauseFetchPartitions(partitionsToPause)
						pausedTopicPartitions[tp] = struct{}{}
					}
				}

				// Now submit to unbounded queue.
				if !consumer.queue.Push(p) {
					// Queue is closed, partition consumer is shutting down
					// Resume the partition if we paused it but won't process
					if _, wasPaused := pausedTopicPartitions[tp]; wasPaused {
						c.client.ResumeFetchPartitions(partitionsToPause)
					}
					return
				}

				if c.testOnlyConfig.fetchTopicPartitionSubmittedCh != nil {
					c.testOnlyConfig.fetchTopicPartitionSubmittedCh <- p
				}
			})

			c.client.AllowRebalance()
		}
	}
}

func (c *KafkaConsumer) reInitClient(ctx context.Context) error {
	var backoffDuration time.Duration = 0
	retries := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		client, err := c.initClient()
		if err != nil {
			retries++
			backoffDuration = getNextBackoffDuration(backoffDuration, retries)
			c.common.log.Error().Err(err).Msg("error initializing Kafka client")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoffDuration):
				continue
			}
		}
		c.client = client
		break
	}
	return nil
}

func (c *KafkaConsumer) assigned(ctx context.Context, cl *kgo.Client, assigned map[string][]int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.doneCh:
		return // Shutdown in progress, skip creating consumers that will be immediately killed.
	default:
	}
	for topic, partitions := range assigned {
		for _, partition := range partitions {
			partitionCtx, cancel := context.WithCancel(ctx)
			pc := &partitionConsumer{
				partitionCtx: partitionCtx,
				cancelFunc:   cancel,
				dispatcher:   c.dispatcher,
				cl:           cl,
				topic:        topic,
				partition:    partition,
				config:       c.config,
				name:         c.name,
				common:       c.common,

				done:  make(chan struct{}),
				queue: newUnboundedQueue(),
			}
			c.consumers[topicPartition{topic, partition}] = pc
			go pc.consume()
		}
	}
}

func (c *KafkaConsumer) revoked(ctx context.Context, cl *kgo.Client, revoked map[string][]int32) {
	c.killConsumers(revoked)
	select {
	case <-c.doneCh:
		// Do not try to CommitMarkedOffsets since on shutdown we call it manually.
	default:
		if err := cl.CommitMarkedOffsets(ctx); err != nil {
			c.common.log.Error().Err(err).Msg("error committing marked offsets on revoke")
		}
	}
}

func (c *KafkaConsumer) lost(_ context.Context, _ *kgo.Client, lost map[string][]int32) {
	c.killConsumers(lost)
	// Losing means we cannot commit: an error happened.
}

func (c *KafkaConsumer) allTopicPartitions() map[string][]int32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string][]int32)
	for tp := range c.consumers {
		result[tp.topic] = append(result[tp.topic], tp.partition)
	}
	return result
}

func (c *KafkaConsumer) killConsumers(lost map[string][]int32) {
	c.mu.Lock()
	var pcs []*partitionConsumer
	for topic, partitions := range lost {
		for _, partition := range partitions {
			tp := topicPartition{topic, partition}
			pc := c.consumers[tp]
			if pc == nil {
				continue
			}
			delete(c.consumers, tp)
			pcs = append(pcs, pc)
		}
	}
	c.mu.Unlock()

	var wg sync.WaitGroup
	for _, pc := range pcs {
		pc.queue.Close()
		pc.cancelFunc()
		wg.Add(1)
		go func() { <-pc.done; wg.Done() }()
	}
	wg.Wait()
}

type partitionConsumer struct {
	partitionCtx context.Context
	cancelFunc   context.CancelFunc
	dispatcher   Dispatcher
	cl           *kgo.Client
	topic        string
	partition    int32
	config       KafkaConfig
	name         string
	common       *consumerCommon

	done  chan struct{}
	queue *unboundedQueue
}

func getUint64HeaderValue(record *kgo.Record, headerKey string) (uint64, error) {
	if headerKey == "" {
		return 0, nil
	}
	headerValue := getHeaderValue(record, headerKey)
	if headerValue == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(headerValue, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing header %s value %s: %w", headerKey, headerValue, err)
	}
	return value, nil
}

func getBoolHeaderValue(record *kgo.Record, headerKey string) (bool, error) {
	if headerKey == "" {
		return false, nil
	}
	headerValue := getHeaderValue(record, headerKey)
	if headerValue == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(headerValue)
	if err != nil {
		return false, fmt.Errorf("error parsing header %s value %s: %w", headerKey, headerValue, err)
	}
	return value, nil
}

func getHeaderValue(record *kgo.Record, headerKey string) string {
	if headerKey == "" {
		return ""
	}
	for _, header := range record.Headers {
		if header.Key == headerKey {
			return string(header.Value)
		}
	}
	return ""
}

func publicationTagsFromKafkaRecord(record *kgo.Record, tagsHeaderPrefix string) map[string]string {
	if tagsHeaderPrefix == "" {
		return nil
	}
	var tags map[string]string
	for _, header := range record.Headers {
		if strings.HasPrefix(header.Key, tagsHeaderPrefix) {
			if tags == nil {
				tags = make(map[string]string)
			}
			tags[header.Key[len(tagsHeaderPrefix):]] = string(header.Value)
		}
	}
	return tags
}

func (pc *partitionConsumer) processPublicationDataRecord(ctx context.Context, record *kgo.Record) error {
	data := record.Value
	idempotencyKey := getHeaderValue(record, pc.config.PublicationDataMode.IdempotencyKeyHeader)
	delta, err := getBoolHeaderValue(record, pc.config.PublicationDataMode.DeltaHeader)
	if err != nil {
		pc.common.log.Error().Err(err).Str("topic", record.Topic).Int32("partition", record.Partition).Msg("error parsing delta header value, skip message")
		return nil
	}
	channelsHeader := getHeaderValue(record, pc.config.PublicationDataMode.ChannelsHeader)
	if channelsHeader == "" {
		pc.common.log.Info().Str("topic", record.Topic).Int32("partition", record.Partition).Msg("no channels found, skip message")
		return nil
	}
	channels := strings.Split(channelsHeader, ",")
	if len(channels) == 0 || (len(channels) == 1 && channels[0] == "") {
		pc.common.log.Info().Str("topic", record.Topic).Int32("partition", record.Partition).Msg("no channels found, skip message")
		return nil
	}
	version, err := getUint64HeaderValue(record, pc.config.PublicationDataMode.VersionHeader)
	if err != nil {
		pc.common.log.Error().Err(err).Str("topic", record.Topic).Int32("partition", record.Partition).Msg("error parsing version header value, skip message")
		return nil
	}
	return pc.dispatcher.DispatchPublication(
		ctx,
		channels,
		api.ConsumedPublication{
			Data:           data,
			IdempotencyKey: idempotencyKey,
			Delta:          delta,
			Tags:           publicationTagsFromKafkaRecord(record, pc.config.PublicationDataMode.TagsHeaderPrefix),
			Version:        version,
			VersionEpoch:   getHeaderValue(record, pc.config.PublicationDataMode.VersionEpochHeader),
		},
	)
}

func (pc *partitionConsumer) processRecord(ctx context.Context, record *kgo.Record) error {
	if pc.config.PublicationDataMode.Enabled {
		return pc.processPublicationDataRecord(ctx, record)
	}
	method := getHeaderValue(record, pc.config.MethodHeader)
	return pc.dispatcher.DispatchCommand(ctx, method, record.Value)
}

func (pc *partitionConsumer) processRecords(records []*kgo.Record) {
	for _, record := range records {
		select {
		case <-pc.partitionCtx.Done():
			return
		default:
		}

		var backoffDuration time.Duration = 0
		retries := 0
		for {
			err := pc.processRecord(pc.partitionCtx, record)
			if err == nil {
				if retries > 0 {
					pc.common.log.Info().Str("topic", record.Topic).Int32("partition", record.Partition).Msg("OK processing message after errors")
				}
				metrics.ConsumerProcessedTotal.WithLabelValues(pc.name).Inc()
				pc.cl.MarkCommitRecords(record)
				break
			}
			if errors.Is(err, context.Canceled) {
				return
			}
			retries++
			backoffDuration = getNextBackoffDuration(backoffDuration, retries)
			metrics.ConsumerErrorsTotal.WithLabelValues(pc.name).Inc()
			pc.common.log.Error().Err(err).Str("topic", record.Topic).Int32("partition", record.Partition).Str("next_attempt_in", backoffDuration.String()).Msg("error processing consumed record")
			select {
			case <-time.After(backoffDuration):
			case <-pc.partitionCtx.Done():
				return
			}
		}
	}
}

func (pc *partitionConsumer) consume() {
	if logging.Enabled(logging.DebugLevel) {
		pc.common.log.Debug().Str("topic", pc.topic).Int32("partition", pc.partition).Msg("starting partition consumer")
	}
	defer close(pc.done)
	defer pc.queue.Close()
	defer func() {
		if logging.Enabled(logging.DebugLevel) {
			// Queue is closed, partition consumer is shutting down.
			pc.common.log.Debug().Str("topic", pc.topic).Int32("partition", pc.partition).Msg("partition consumer shutting down")
		}
	}()

	partitionsToResume := map[string][]int32{pc.topic: {pc.partition}}
	resumeConsuming := func() {
		pc.cl.ResumeFetchPartitions(partitionsToResume)
	}
	defer resumeConsuming()

	for {
		select {
		case <-pc.partitionCtx.Done():
			return
		case <-pc.queue.NotEmpty():
			if pc.queue.IsClosed() {
				return
			}
			// Process all available items in the queue.
			for {
				select {
				case <-pc.partitionCtx.Done():
					return
				default:
				}
				p, ok := pc.queue.Pop()
				if !ok {
					break
				}
				pc.processRecords(p.Records)
				// Resume partition after processing this batch. Only one additional batch will
				// be added to the queue before we pause again.
				resumeConsuming()
			}
		}
	}
}

// unboundedQueue implements an unbounded queue for FetchTopicPartition
type unboundedQueue struct {
	mu         sync.RWMutex
	items      []kgo.FetchTopicPartition
	numRecords int
	notEmpty   chan struct{}
	closed     bool
}

func newUnboundedQueue() *unboundedQueue {
	return &unboundedQueue{
		items:    make([]kgo.FetchTopicPartition, 0),
		notEmpty: make(chan struct{}, 1),
	}
}

func (q *unboundedQueue) Push(item kgo.FetchTopicPartition) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return false
	}

	q.items = append(q.items, item)
	q.numRecords += len(item.Records)

	// Signal that queue is not empty.
	select {
	case q.notEmpty <- struct{}{}:
	default:
	}

	return true
}

func (q *unboundedQueue) Pop() (kgo.FetchTopicPartition, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed || len(q.items) == 0 {
		return kgo.FetchTopicPartition{}, false
	}

	// We do not expect many items in the queue, so we can use a simple slice pop operation and
	// not worry about memory being reserved for the underlying slice.
	item := q.items[0]
	q.items[0] = kgo.FetchTopicPartition{} // Clear the first item for faster GC.
	q.items = q.items[1:]
	q.numRecords -= len(item.Records)

	return item, true
}

func (q *unboundedQueue) NumRecords() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.numRecords
}

func (q *unboundedQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if !q.closed {
		q.closed = true
		close(q.notEmpty)
	}
}

func (q *unboundedQueue) IsClosed() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.closed
}

func (q *unboundedQueue) NotEmpty() <-chan struct{} {
	return q.notEmpty
}
