package consuming

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

type KafkaConfig struct {
	Brokers        []string `mapstructure:"brokers" json:"brokers"`
	Topics         []string `mapstructure:"topics" json:"topics"`
	ConsumerGroup  string   `mapstructure:"consumer_group" json:"consumer_group"`
	MaxPollRecords int      `mapstructure:"max_poll_records" json:"max_poll_records"`

	// TLS may be enabled, and mTLS auth may be configured.
	TLS              bool `mapstructure:"tls" json:"tls"`
	tools.TLSOptions `mapstructure:",squash"`

	// SASLMechanism when not empty enables SASL auth. For now, Centrifugo only
	// supports "plain" SASL mechanism.
	SASLMechanism string `mapstructure:"sasl_mechanism" json:"sasl_mechanism"`
	SASLUser      string `mapstructure:"sasl_user" json:"sasl_user"`
	SASLPassword  string `mapstructure:"sasl_password" json:"sasl_password"`
}

type topicPartition struct {
	t string
	p int32
}

type KafkaConsumer struct {
	name       string
	client     *kgo.Client
	nodeID     string
	logger     Logger
	dispatcher Dispatcher
	config     KafkaConfig
	consumers  map[topicPartition]*partitionConsumer
	doneCh     chan struct{}
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

func NewKafkaConsumer(name string, nodeID string, logger Logger, dispatcher Dispatcher, config KafkaConfig) (*KafkaConsumer, error) {
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
	consumer := &KafkaConsumer{
		name:       name,
		nodeID:     nodeID,
		logger:     logger,
		dispatcher: dispatcher,
		config:     config,
		consumers:  make(map[topicPartition]*partitionConsumer),
		doneCh:     make(chan struct{}),
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
	if c.config.TLS {
		tlsOptionsMap, err := c.config.TLSOptions.ToMap()
		if err != nil {
			return nil, fmt.Errorf("error in TLS configuration: %w", err)
		}
		tlsConfig, err := tools.MakeTLSConfig(tlsOptionsMap, "", os.ReadFile)
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
		if c.config.SASLMechanism != "plain" {
			return nil, fmt.Errorf("only plain SASL auth mechanism is supported")
		}
		opts = append(opts, kgo.SASL(plain.Auth{
			User: c.config.SASLUser,
			Pass: c.config.SASLPassword,
		}.AsMechanism()))
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
				c.logger.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "commit marked offsets error on shutdown", map[string]any{"error": err.Error()}))
			}
			err := c.leaveGroup(closeCtx, c.client)
			if err != nil {
				c.logger.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error leaving consumer group", map[string]any{"error": err.Error()}))
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
			c.logger.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error polling Kafka", map[string]any{"error": err.Error()}))
		}
		// Upon returning from polling loop we are re-initializing consumer client.
		c.client.CloseAllowingRebalance()
		c.client = nil
		c.logger.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "start re-initializing Kafka consumer client", map[string]any{}))
		err = c.reInitClient(ctx)
		if err != nil {
			// Only context.Canceled may be returned.
			return err
		}
		c.logger.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "Kafka consumer client re-initialized", map[string]any{}))
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
					c.logger.Log(centrifuge.NewLogEntry(
						centrifuge.LogLevelError, "error while polling Kafka",
						map[string]any{"error": fetchErr.Err.Error(), "topic": fetchErr.Topic, "partition": fetchErr.Partition}),
					)
				}
				return fmt.Errorf("poll error: %w", errors.Join(errs...))
			}

			fetches.EachPartition(func(p kgo.FetchTopicPartition) {
				if len(p.Records) == 0 {
					return
				}

				tp := topicPartition{p.Topic, p.Partition}

				partitionsToPause := map[string][]int32{p.Topic: {p.Partition}}
				// PauseFetchPartitions here to not poll partition until records are processed.
				// This allows parallel processing of records from different partitions, without
				// keeping records in memory and blocking rebalance. Resume will be called after
				// records are processed by c.consumers[tp].
				c.client.PauseFetchPartitions(partitionsToPause)

				// resumeConsuming is a helper function to be called if context is done or partition
				// consumer is closed before records were sent to it. It's not strictly necessary
				// to resume consuming upon context done since we do not re-use the client after that,
				// but it seems a good thing to do from the cleanup perspective.
				resumeConsuming := func() {
					c.client.ResumeFetchPartitions(partitionsToPause)
				}

				// Since we are using BlockRebalanceOnPoll, we can be
				// sure this partition consumer exists:
				// * onAssigned is guaranteed to be called before we
				// fetch offsets for newly added partitions
				// * onRevoked waits for partition consumers to quit
				// and be deleted before re-allowing polling.
				select {
				case <-ctx.Done():
					resumeConsuming()
					return
				case <-c.consumers[tp].quit:
					resumeConsuming()
					return
				case c.consumers[tp].recs <- p:
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
			c.logger.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error initializing Kafka client", map[string]any{"error": err.Error()}))
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
	for topic, partitions := range assigned {
		for _, partition := range partitions {
			pc := &partitionConsumer{
				clientCtx:  ctx,
				dispatcher: c.dispatcher,
				logger:     c.logger,
				cl:         cl,
				topic:      topic,
				partition:  partition,

				quit: make(chan struct{}),
				done: make(chan struct{}),
				recs: make(chan kgo.FetchTopicPartition),
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
			c.logger.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "commit error on revoked partitions", map[string]any{"error": err.Error()}))
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
	clientCtx  context.Context
	dispatcher Dispatcher
	logger     Logger
	cl         *kgo.Client
	topic      string
	partition  int32

	quit chan struct{}
	done chan struct{}
	recs chan kgo.FetchTopicPartition
}

func (pc *partitionConsumer) processRecords(records []*kgo.Record) {
	for _, record := range records {
		select {
		case <-pc.clientCtx.Done():
			return
		case <-pc.quit:
			return
		default:
		}

		var e KafkaJSONEvent
		err := json.Unmarshal(record.Value, &e)
		if err != nil {
			pc.logger.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error unmarshalling event from Kafka", map[string]any{"error": err.Error(), "topic": record.Topic, "partition": record.Partition}))
			pc.cl.MarkCommitRecords(record)
			continue
		}

		var backoffDuration time.Duration = 0
		retries := 0
		for {
			err := pc.dispatcher.Dispatch(pc.clientCtx, e.Method, e.Payload)
			if err == nil {
				if retries > 0 {
					pc.logger.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "OK processing events after errors", map[string]any{}))
				}
				pc.cl.MarkCommitRecords(record)
				break
			}
			retries++
			backoffDuration = getNextBackoffDuration(backoffDuration, retries)
			pc.logger.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error processing consumed event", map[string]any{"error": err.Error(), "method": e.Method, "nextAttemptIn": backoffDuration.String()}))
			select {
			case <-time.After(backoffDuration):
			case <-pc.quit:
				return
			case <-pc.clientCtx.Done():
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
		case <-pc.clientCtx.Done():
			return
		case <-pc.quit:
			return
		case p := <-pc.recs:
			pc.processRecords(p.Records)
			// At this point we are ready to consume the next batch from partition, thus resume.
			resumeConsuming()
		}
	}
}
