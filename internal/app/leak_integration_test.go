//go:build integration

package app

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/rueidis"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/goleak"
)

const (
	integrationPGDSN    = "postgres://test:test@localhost:5432/test"
	integrationKafka    = "localhost:29092"
	integrationNATS     = "nats://localhost:4222"
	integrationRedis    = "localhost:6379"
	integrationPubSub   = "localhost:8085"
	integrationSQS      = "http://localhost:4566"
	integrationProject  = "test-project"
	integrationRegion   = "us-east-1"
	integrationChannel  = "test"
	integrationAPIKey   = "api-key"
	integrationHMAC     = "secret"
	publishCommandValue = `{"channel":"test","data":{"text":"consumed"}}`
)

// integrationConfig is a config serving uni HTTP streams and the server API,
// with extra top-level sections.
func integrationConfig(extra string) string {
	cfg := `{
		"client": {"token": {"hmac_secret_key": "` + integrationHMAC + `"}},
		"http_api": {"key": "` + integrationAPIKey + `"},
		"uni_http_stream": {"enabled": true},
		"usage_stats": {"disabled": true}`
	if extra != "" {
		cfg += ", " + extra
	}
	return cfg + "}"
}

// runTwice runs an App from cfg twice with traffic: a client receives a
// publication sent through the server API and, when produce is set, one
// produced to a consumer. Nothing must remain running afterwards.
func runTwice(t *testing.T, cfg string, produce func(t *testing.T)) {
	t.Helper()
	for i := 0; i < 2; i++ {
		a, err := newTestApp(t, cfg)
		require.NoError(t, err)
		require.NoError(t, a.Start())
		addr := httpAddr(t, a)
		stream := connectUniHTTPStream(t, addr, "user", integrationChannel)
		apiPublish(t, testHTTPClient(t), addr, integrationChannel, `{"text":"hello"}`)
		require.Equal(t, integrationChannel, stream.next(t).Channel)
		if produce != nil {
			produce(t)
			for {
				push := stream.next(t)
				if strings.Contains(string(push.Pub.Data), "consumed") {
					break
				}
			}
		}
		_ = stream.Close()
		code, err := a.Shutdown(ExitStop, 0)
		require.NoError(t, err)
		require.Equal(t, 0, code)
		a.Close()
	}
}

func TestIntegrationLifecycleRedisNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)
	prefix := "leak" + uuid.NewString()[:8]
	runTwice(t, integrationConfig(`
		"engine": {"type": "redis", "redis": {"address": "`+integrationRedis+`", "prefix": "`+prefix+`"}},
		"map_broker": {"type": "redis", "redis": {"address": "`+integrationRedis+`", "prefix": "`+prefix+`"}}`), nil)
}

func TestIntegrationLifecycleRedisBrokerAndPresenceNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)
	prefix := "leak" + uuid.NewString()[:8]
	runTwice(t, integrationConfig(`
		"broker": {"enabled": true, "type": "redis", "redis": {"address": "`+integrationRedis+`", "prefix": "`+prefix+`"}},
		"presence_manager": {"enabled": true, "type": "redis", "redis": {"address": "`+integrationRedis+`", "prefix": "`+prefix+`"}}`), nil)
}

func TestIntegrationLifecyclePostgresNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)
	prefix := "leak" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	runTwice(t, integrationConfig(`
		"broker": {"enabled": true, "type": "postgres", "postgres": {"dsn": "`+integrationPGDSN+`", "table_prefix": "`+prefix+`b"}},
		"map_broker": {"type": "postgres", "postgres": {"dsn": "`+integrationPGDSN+`", "table_prefix": "`+prefix+`m"}},
		"controller": {"enabled": true, "type": "postgres", "postgres": {"dsn": "`+integrationPGDSN+`", "table_prefix": "`+prefix+`c"}}`), nil)
}

func TestIntegrationLifecycleRedisStreamConsumerNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)
	stream := "leak_stream_" + uuid.NewString()
	client, err := rueidis.NewClient(rueidis.ClientOption{InitAddress: []string{integrationRedis}})
	require.NoError(t, err)
	defer client.Close()
	runTwice(t, integrationConfig(`"consumers": [{"name": "redis", "type": "redis_stream", "enabled": true,
		"redis_stream": {"address": "`+integrationRedis+`", "streams": ["`+stream+`"], "consumer_group": "leak", "num_workers": 2}}]`),
		func(t *testing.T) {
			cmd := client.B().Xadd().Key(stream).Id("*").FieldValue().
				FieldValue("method", "publish").FieldValue("payload", publishCommandValue).Build()
			require.NoError(t, client.Do(context.Background(), cmd).Error())
		})
}

func TestIntegrationLifecyclePostgresConsumerNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)
	table := "leak_outbox_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	pool, err := pgxpool.New(context.Background(), integrationPGDSN)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(context.Background(), `CREATE TABLE `+table+` (
		id BIGSERIAL PRIMARY KEY, method text NOT NULL, payload BYTEA NOT NULL, partition INTEGER NOT NULL)`)
	require.NoError(t, err)
	defer func() { _, _ = pool.Exec(context.Background(), "DROP TABLE "+table) }()
	runTwice(t, integrationConfig(`"consumers": [{"name": "pg", "type": "postgresql", "enabled": true,
		"postgresql": {"dsn": "`+integrationPGDSN+`", "outbox_table_name": "`+table+`", "num_partitions": 1}}]`),
		func(t *testing.T) {
			_, err := pool.Exec(context.Background(), "INSERT INTO "+table+" (method, payload, partition) VALUES ($1, $2, 0)",
				"publish", []byte(publishCommandValue))
			require.NoError(t, err)
		})
}

func TestIntegrationLifecycleNatsJetStreamConsumerNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)
	subject := "leak.subject." + uuid.NewString()
	streamName := "LEAK_STREAM_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	nc, err := nats.Connect(integrationNATS)
	require.NoError(t, err)
	defer nc.Close()
	js, err := nc.JetStream()
	require.NoError(t, err)
	_, err = js.AddStream(&nats.StreamConfig{Name: streamName, Subjects: []string{subject}, Storage: nats.MemoryStorage})
	require.NoError(t, err)
	defer func() { _ = js.DeleteStream(streamName) }()
	runTwice(t, integrationConfig(`"consumers": [{"name": "nats", "type": "nats_jetstream", "enabled": true,
		"nats_jetstream": {"url": "`+integrationNATS+`", "stream_name": "`+streamName+`", "subjects": ["`+subject+`"],
		"durable_consumer_name": "leak"}}]`),
		func(t *testing.T) {
			msg := nats.NewMsg(subject)
			msg.Header.Set("centrifugo-method", "publish")
			msg.Data = []byte(publishCommandValue)
			_, err := js.PublishMsg(msg)
			require.NoError(t, err)
		})
}

func TestIntegrationLifecycleKafkaConsumerNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)
	topic := "leak_topic_" + uuid.NewString()
	cl, err := kgo.NewClient(kgo.SeedBrokers(integrationKafka))
	require.NoError(t, err)
	defer cl.Close()
	adm := kadm.NewClient(cl)
	_, err = adm.CreateTopics(context.Background(), 1, 1, nil, topic)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		details, err := adm.ListTopics(context.Background(), topic)
		if err != nil {
			return false
		}
		td, ok := details[topic]
		return ok && td.Err == nil && len(td.Partitions) == 1 && td.Partitions[0].Leader >= 0
	}, 15*time.Second, 50*time.Millisecond)
	runTwice(t, integrationConfig(`"consumers": [{"name": "kafka", "type": "kafka", "enabled": true,
		"kafka": {"brokers": ["`+integrationKafka+`"], "topics": ["`+topic+`"], "consumer_group": "leak"}}]`),
		func(t *testing.T) {
			res := cl.ProduceSync(context.Background(), &kgo.Record{
				Topic:   topic,
				Value:   []byte(publishCommandValue),
				Headers: []kgo.RecordHeader{{Key: "centrifugo-method", Value: []byte("publish")}},
			})
			require.NoError(t, res.FirstErr())
		})
}

func TestIntegrationLifecycleGooglePubSubConsumerNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, append(leakOptions(),
		// The Pub/Sub library checks for a closed stream only between ticks of
		// its keep-alive tickers: the goroutine returns at the next tick.
		goleak.IgnoreTopFunction("cloud.google.com/go/pubsub/v2.(*messageIterator).streamKeepAliveHandler"),
	)...)
	t.Setenv("PUBSUB_EMULATOR_HOST", integrationPubSub)
	ctx := context.Background()
	topicID := "leak-topic-" + uuid.NewString()
	subID := "leak-sub-" + uuid.NewString()
	client, err := pubsub.NewClient(ctx, integrationProject)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()
	topic := fmt.Sprintf("projects/%s/topics/%s", integrationProject, topicID)
	_, err = client.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{Name: topic})
	require.NoError(t, err)
	_, err = client.SubscriptionAdminClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:  fmt.Sprintf("projects/%s/subscriptions/%s", integrationProject, subID),
		Topic: topic,
	})
	require.NoError(t, err)
	publisher := client.Publisher(topic)
	defer publisher.Stop()
	runTwice(t, integrationConfig(`"consumers": [{"name": "pubsub", "type": "google_pub_sub", "enabled": true,
		"google_pub_sub": {"project_id": "`+integrationProject+`", "subscriptions": ["`+subID+`"]}}]`),
		func(t *testing.T) {
			_, err := publisher.Publish(ctx, &pubsub.Message{
				Data:       []byte(publishCommandValue),
				Attributes: map[string]string{"centrifugo-method": "publish"},
			}).Get(ctx)
			require.NoError(t, err)
		})
}

func TestIntegrationLifecycleAwsSqsConsumerNoLeaks(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	defer goleak.VerifyNone(t, append(leakOptions(),
		// SQS clients (this test's and the consumer's) keep idle keep-alive
		// connections, closed by their idle timeout: the SDK client has no API
		// to close them.
		goleak.IgnoreAnyFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreAnyFunction("net/http.(*persistConn).writeLoop"),
	)...)
	ctx := context.Background()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(integrationRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")))
	require.NoError(t, err)
	endpoint, _ := url.Parse(integrationSQS)
	sqsClient := sqs.NewFromConfig(awsCfg, sqs.WithEndpointResolverV2(staticSQSResolver{endpoint: *endpoint}))
	queueName := "leak-queue-" + uuid.NewString()
	out, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: &queueName})
	require.NoError(t, err)
	queueURL := *out.QueueUrl
	runTwice(t, integrationConfig(`"consumers": [{"name": "sqs", "type": "aws_sqs", "enabled": true,
		"aws_sqs": {"region": "`+integrationRegion+`", "queues": ["`+queueURL+`"], "localstack_endpoint": "`+integrationSQS+`",
		"max_number_of_messages": 10}}]`),
		func(t *testing.T) {
			_, err := sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
				QueueUrl:    &queueURL,
				MessageBody: aws.String(publishCommandValue),
				MessageAttributes: map[string]types.MessageAttributeValue{
					"centrifugo-method": {DataType: aws.String("String"), StringValue: aws.String("publish")},
				},
			})
			require.NoError(t, err)
		})
}

type staticSQSResolver struct {
	endpoint url.URL
}

func (r staticSQSResolver) ResolveEndpoint(context.Context, sqs.EndpointParameters) (smithyendpoints.Endpoint, error) {
	return smithyendpoints.Endpoint{URI: r.endpoint}, nil
}
