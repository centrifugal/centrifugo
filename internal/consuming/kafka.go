package consuming

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/apiproto"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/rs/zerolog/log"
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
}

type topicPartition struct {
	topic     string
	partition int32
}

type KafkaConsumer struct {
	name           string
	client         *kgo.Client
	nodeID         string
	dispatcher     Dispatcher
	config         KafkaConfig
	consumers      map[topicPartition]*partitionConsumer
	doneCh         chan struct{}
	metrics        *commonMetrics
	testOnlyConfig testOnlyConfig
}

// JSONRawOrString can decode payload from bytes and from JSON string. This gives
// us better interoperability. For example, JSONB field is encoded as JSON string in
// Debezium PostgreSQL connector.
type JSONRawOrString json.RawMessage

func (j *JSONRawOrString) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		// Unmarshal as a string, then convert the string to json.RawMessage.
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		*j = JSONRawOrString(str)
	} else {
		// Unmarshal directly as json.RawMessage
		*j = data
	}
	return nil
}

// MarshalJSON returns m as the JSON encoding of m.
func (j JSONRawOrString) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return j, nil
}

type KafkaJSONEvent struct {
	Method  string          `json:"method"`
	Payload JSONRawOrString `json:"payload"`
}

func NewKafkaConsumer(
	name string, nodeID string, dispatcher Dispatcher, config KafkaConfig, metrics *commonMetrics,
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
	if config.PartitionBufferSize < 0 {
		return nil, errors.New("partition buffer size can't be negative")
	}
	consumer := &KafkaConsumer{
		name:       name,
		nodeID:     nodeID,
		dispatcher: dispatcher,
		config:     config,
		consumers:  make(map[topicPartition]*partitionConsumer),
		doneCh:     make(chan struct{}),
		metrics:    metrics,
	}
	cl, err := consumer.initClient()
	if err != nil {
		return nil, fmt.Errorf("error init Kafka client: %w", err)
	}
	consumer.client = cl
	return consumer, nil
}

const kafkaClientID = "centrifugo"

func (c *KafkaConsumer) getInstanceID() string {
	return "centrifugo-" + c.nodeID
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
		kgo.InstanceID(c.getInstanceID()),
	}
	if c.config.FetchMaxBytes > 0 {
		opts = append(opts, kgo.FetchMaxBytes(c.config.FetchMaxBytes))
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

func (c *KafkaConsumer) leaveGroup(ctx context.Context, client *kgo.Client) error {
	req := kmsg.NewPtrLeaveGroupRequest()
	req.Group = c.config.ConsumerGroup
	instanceID := c.getInstanceID()
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

func (c *KafkaConsumer) Run(ctx context.Context) error {
	defer func() {
		close(c.doneCh)
		if c.client != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			// The reason we make CommitMarkedOffsets here is because franz-go does not send
			// LeaveGroup request when instanceID is used. So we leave manually. But we have
			// to commit what we have at this point. The closed doneCh then allows partition
			// consumers to skip calling CommitMarkedOffsets on revoke. Otherwise, we get
			// "UNKNOWN_MEMBER_ID" error (since group already left).
			if err := c.client.CommitMarkedOffsets(closeCtx); err != nil {
				log.Error().Err(err).Str("consumer", c.name).Msg("error committing marked offsets on shutdown")
			}
			err := c.leaveGroup(closeCtx, c.client)
			if err != nil {
				log.Error().Err(err).Str("consumer", c.name).Msg("error leaving consumer group")
			}
			c.client.CloseAllowingRebalance()
		}
	}()
	for {
		err := c.pollUntilFatal(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return ctx.Err()
			}
			log.Error().Err(err).Str("consumer", c.name).Msg("error polling Kafka")
		}
		// Upon returning from polling loop we are re-initializing consumer client.
		c.client.CloseAllowingRebalance()
		c.client = nil
		log.Info().Str("consumer", c.name).Msg("re-initializing Kafka consumer client")
		err = c.reInitClient(ctx)
		if err != nil {
			// Only context.Canceled may be returned.
			return err
		}
		log.Info().Str("consumer", c.name).Msg("Kafka consumer client re-initialized")
	}
}

func (c *KafkaConsumer) pollUntilFatal(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// PollRecords is recommended when using BlockRebalanceOnPoll.
			// Need to ensure that processor loop complete fast enough to not block a rebalance for too long.
			fetches := c.client.PollRecords(ctx, c.config.MaxPollRecords)
			if fetches.IsClientClosed() {
				return nil
			}
			fetchErrors := fetches.Errors()
			if len(fetchErrors) > 0 {
				// Non-retriable errors returned. We will restart consumer client, but log errors first.
				var errs []error
				for _, fetchErr := range fetchErrors {
					if errors.Is(fetchErr.Err, context.Canceled) {
						return ctx.Err()
					}
					errs = append(errs, fetchErr.Err)
					log.Error().Err(fetchErr.Err).Str("topic", fetchErr.Topic).Int32("partition", fetchErr.Partition).Msg("error while polling Kafka")
				}
				return fmt.Errorf("poll error: %w", errors.Join(errs...))
			}

			pausedTopicPartitions := map[topicPartition]struct{}{}
			fetches.EachPartition(func(p kgo.FetchTopicPartition) {
				if len(p.Records) == 0 {
					return
				}

				tp := topicPartition{p.Topic, p.Partition}
				if _, paused := pausedTopicPartitions[tp]; paused {
					// We have already paused this partition during this poll, so we should not
					// process records from it anymore. We will resume partition processing with the
					// correct offset soon, after we have space in recs buffer.
					return
				}

				// Since we are using BlockRebalanceOnPoll, we can be
				// sure this partition consumer exists:
				// * onAssigned is guaranteed to be called before we
				// fetch offsets for newly added partitions
				// * onRevoked waits for partition consumers to quit
				// and be deleted before re-allowing polling.
				select {
				case <-ctx.Done():
					return
				case <-c.consumers[tp].quit:
					return
				case c.consumers[tp].recs <- p:
					if c.testOnlyConfig.fetchTopicPartitionSubmittedCh != nil { // Only set in tests.
						c.testOnlyConfig.fetchTopicPartitionSubmittedCh <- p
					}
				default:
					if c.testOnlyConfig.topicPartitionBeforePauseCh != nil { // Only set in tests.
						c.testOnlyConfig.topicPartitionBeforePauseCh <- tp
					}
					if c.testOnlyConfig.topicPartitionPauseProceedCh != nil { // Only set in tests.
						<-c.testOnlyConfig.topicPartitionPauseProceedCh
					}

					partitionsToPause := map[string][]int32{p.Topic: {p.Partition}}
					// PauseFetchPartitions here to not poll partition until records are processed.
					// This allows parallel processing of records from different partitions, without
					// keeping records in memory and blocking rebalance. Resume will be called after
					// records are processed by c.consumers[tp].
					c.client.PauseFetchPartitions(partitionsToPause)
					defer func() {
						// There is a chance that message processor resumed partition processing before
						// we called PauseFetchPartitions above. Such a race was observed in a service
						// under CPU throttling conditions. In that case Pause is called after Resume,
						// and topic is never resumed after that. To avoid we check if the channel current
						// len is less than cap, if len < cap => we can be sure that buffer has space now,
						// so we can resume partition processing. If it is not, this means that the records
						// are still not processed and resume will be called eventually after processing by
						// partition consumer. See also TestKafkaConsumer_TestPauseAfterResumeRace test case.
						if len(c.consumers[tp].recs) < cap(c.consumers[tp].recs) {
							c.client.ResumeFetchPartitions(partitionsToPause)
						}
					}()

					pausedTopicPartitions[tp] = struct{}{}
					// To poll next time since correct offset we need to set it manually to the offset of
					// the first record in the batch. Otherwise, next poll will return the next record batch,
					// and we will lose the current one.
					epochOffset := kgo.EpochOffset{Epoch: -1, Offset: p.Records[0].Offset}
					c.client.SetOffsets(map[string]map[int32]kgo.EpochOffset{p.Topic: {p.Partition: epochOffset}})
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
			log.Error().Err(err).Str("consumer", c.name).Msg("error initializing Kafka client")
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

const defaultPartitionBufferSize = 8

func (c *KafkaConsumer) assigned(ctx context.Context, cl *kgo.Client, assigned map[string][]int32) {
	bufferSize := c.config.PartitionBufferSize
	if bufferSize == 0 {
		bufferSize = defaultPartitionBufferSize
	}
	for topic, partitions := range assigned {
		for _, partition := range partitions {
			quitCh := make(chan struct{})
			partitionCtx, cancel := context.WithCancel(ctx)
			go func() {
				select {
				case <-ctx.Done():
					cancel()
				case <-quitCh:
					cancel()
				}
			}()
			pc := &partitionConsumer{
				partitionCtx: partitionCtx,
				dispatcher:   c.dispatcher,
				cl:           cl,
				topic:        topic,
				partition:    partition,
				config:       c.config,
				name:         c.name,
				metrics:      c.metrics,

				quit: quitCh,
				done: make(chan struct{}),
				recs: make(chan kgo.FetchTopicPartition, bufferSize),
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
			log.Error().Err(err).Str("consumer", c.name).Msg("error committing marked offsets on revoke")
		}
	}
}

func (c *KafkaConsumer) lost(_ context.Context, _ *kgo.Client, lost map[string][]int32) {
	c.killConsumers(lost)
	// Losing means we cannot commit: an error happened.
}

func (c *KafkaConsumer) killConsumers(lost map[string][]int32) {
	var wg sync.WaitGroup
	defer wg.Wait()

	for topic, partitions := range lost {
		for _, partition := range partitions {
			tp := topicPartition{topic, partition}
			pc := c.consumers[tp]
			delete(c.consumers, tp)
			close(pc.quit)
			wg.Add(1)
			go func() { <-pc.done; wg.Done() }()
		}
	}
}

type partitionConsumer struct {
	partitionCtx context.Context
	dispatcher   Dispatcher
	cl           *kgo.Client
	topic        string
	partition    int32
	config       KafkaConfig
	name         string
	metrics      *commonMetrics

	quit chan struct{}
	done chan struct{}
	recs chan kgo.FetchTopicPartition
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

func (pc *partitionConsumer) processPublicationDataRecord(ctx context.Context, record *kgo.Record) error {
	data := record.Value
	idempotencyKey := getHeaderValue(record, pc.config.PublicationDataMode.IdempotencyKeyHeader)
	var delta bool
	if pc.config.PublicationDataMode.DeltaHeader != "" {
		var err error
		delta, err = strconv.ParseBool(getHeaderValue(record, pc.config.PublicationDataMode.DeltaHeader))
		if err != nil {
			log.Error().Err(err).Str("topic", record.Topic).Int32("partition", record.Partition).Msg("error parsing delta header value, skip message")
			return nil
		}
	}
	channels := strings.Split(getHeaderValue(record, pc.config.PublicationDataMode.ChannelsHeader), ",")
	if len(channels) == 0 {
		log.Info().Str("consumer_name", pc.name).Str("topic", record.Topic).Int32("partition", record.Partition).Msg("no channels found, skip message")
		return nil
	}
	if len(channels) == 1 {
		req := &apiproto.PublishRequest{
			Data:           data,
			Channel:        channels[0],
			IdempotencyKey: idempotencyKey,
			Delta:          delta,
		}
		return pc.dispatcher.Publish(ctx, req)
	}
	req := &apiproto.BroadcastRequest{
		Data:           data,
		Channels:       channels,
		IdempotencyKey: idempotencyKey,
		Delta:          delta,
	}
	return pc.dispatcher.Broadcast(ctx, req)
}

func (pc *partitionConsumer) processAPICommandRecord(ctx context.Context, record *kgo.Record) error {
	var e KafkaJSONEvent
	err := json.Unmarshal(record.Value, &e)
	if err != nil {
		log.Error().Err(err).Str("consumer_name", pc.name).Str("topic", record.Topic).Int32("partition", record.Partition).Msg("error unmarshalling event from Kafka, skip message")
		return nil
	}
	return pc.dispatcher.Dispatch(ctx, e.Method, e.Payload)
}

func (pc *partitionConsumer) processRecord(ctx context.Context, record *kgo.Record) error {
	if pc.config.PublicationDataMode.Enabled {
		return pc.processPublicationDataRecord(ctx, record)
	}
	return pc.processAPICommandRecord(ctx, record)
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
					log.Info().Str("consumer_name", pc.name).Str("topic", record.Topic).Int32("partition", record.Partition).Msg("OK processing message after errors")
				}
				pc.metrics.processedTotal.WithLabelValues(pc.name).Inc()
				pc.cl.MarkCommitRecords(record)
				break
			}
			retries++
			backoffDuration = getNextBackoffDuration(backoffDuration, retries)
			pc.metrics.errorsTotal.WithLabelValues(pc.name).Inc()
			log.Error().Err(err).Str("consumer_name", pc.name).Str("topic", record.Topic).Int32("partition", record.Partition).Str("next_attempt_in", backoffDuration.String()).Msg("error processing consumed record")
			select {
			case <-time.After(backoffDuration):
			case <-pc.partitionCtx.Done():
				return
			}
		}
	}
}

func (pc *partitionConsumer) consume() {
	defer close(pc.done)
	partitionsToResume := map[string][]int32{pc.topic: {pc.partition}}
	resumeConsuming := func() {
		pc.cl.ResumeFetchPartitions(partitionsToResume)
	}
	defer resumeConsuming()
	for {
		select {
		case <-pc.partitionCtx.Done():
			return
		case p := <-pc.recs:
			pc.processRecords(p.Records)
			// After processing records, we can resume partition processing (if it was paused, no-op otherwise).
			resumeConsuming()
		}
	}
}
