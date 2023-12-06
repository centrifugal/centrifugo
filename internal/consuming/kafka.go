package consuming

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

type KafkaConfig struct {
	Brokers       []string `mapstructure:"brokers" json:"brokers"`
	Topics        []string `mapstructure:"topics" json:"topics"`
	ConsumerGroup string   `mapstructure:"consumer_group" json:"consumer_group"`
}

type topicPartition struct {
	t string
	p int32
}

type KafkaConsumer struct {
	client     *kgo.Client
	nodeID     string
	logger     Logger
	dispatcher Dispatcher
	config     KafkaConfig
	consumers  map[topicPartition]*partitionConsumer
}

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
func (m JSONRawOrString) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return m, nil
}

type KafkaJSONEvent struct {
	Method  string          `json:"method"`
	Payload JSONRawOrString `json:"payload"`
}

func NewKafkaConsumer(nodeID string, logger Logger, dispatcher Dispatcher, config KafkaConfig) (*KafkaConsumer, error) {
	consumer := &KafkaConsumer{
		nodeID:     nodeID,
		logger:     logger,
		dispatcher: dispatcher,
		config:     config,
		consumers:  make(map[topicPartition]*partitionConsumer),
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
	client, err := kgo.NewClient(
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
	)
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
		if c.client != nil {
			leaveCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := c.leaveGroup(leaveCtx, c.client)
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
			// PollRecords is recommended when using BlockRebalanceOnPoll. You can tune how many records to
			// Need to ensure that processor loop complete fast enough to not block a rebalance for too long.
			fetches := c.client.PollRecords(ctx, 1000)
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
				tp := topicPartition{p.Topic, p.Partition}

				// Since we are using BlockRebalanceOnPoll, we can be
				// sure this partition consumer exists:
				//
				// * onAssigned is guaranteed to be called before we
				// fetch offsets for newly added partitions
				//
				// * onRevoked waits for partition consumers to quit
				// and be deleted before re-allowing polling.
				select {
				case <-ctx.Done():
					return
				case <-c.consumers[tp].quit:
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
	if err := cl.CommitMarkedOffsets(ctx); err != nil {
		c.logger.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "revoke commit error", map[string]any{"error": err.Error()}))
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

func (pc *partitionConsumer) consume() {
	defer close(pc.done)
	//fmt.Printf("starting, t %s p %d\n", pc.topic, pc.partition)
	//defer fmt.Printf("killing, t %s p %d\n", pc.topic, pc.partition)
	for {
		select {
		case <-pc.clientCtx.Done():
			return
		case <-pc.quit:
			return
		case p := <-pc.recs:
			for _, record := range p.Records {
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
					pc.logger.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error processing events", map[string]any{"error": err.Error(), "nextAttemptIn": backoffDuration.String()}))
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
	}
}
