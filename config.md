# `consumers`

Type: `[]Consumer` object. Default: `[]`

Env: `CENTRIFUGO_CONSUMERS`

`consumers` is a configuration for message queue consumers. For example, Centrifugo can consume
messages from PostgreSQL transactional outbox table, or from Kafka topics.

## `consumers[].name`

Type: `string`

`name` is a unique name required for each consumer.

## `consumers[].enabled`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_ENABLED`

`enabled` must be true to tell Centrifugo to run configured consumer.

## `consumers[].type`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_TYPE`

`type` describes the type of consumer. Supported types are: `postgresql`, `kafka`, `nats_jetstream`,
`redis_stream`, `google_pub_sub`, `aws_sqs`, `azure_service_bus`.

## `consumers[].postgresql`

Type: `PostgresConsumerConfig` object

`postgresql` allows defining options for consumer of postgresql type.

### `consumers[].postgresql.dsn`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_POSTGRESQL_DSN`

No documentation available.

### `consumers[].postgresql.outbox_table_name`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_POSTGRESQL_OUTBOX_TABLE_NAME`

No documentation available.

### `consumers[].postgresql.num_partitions`

Type: `int`. Default: `1`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_POSTGRESQL_NUM_PARTITIONS`

No documentation available.

### `consumers[].postgresql.partition_select_limit`

Type: `int`. Default: `100`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_POSTGRESQL_PARTITION_SELECT_LIMIT`

No documentation available.

### `consumers[].postgresql.partition_poll_interval`

Type: `Duration`. Default: `300ms`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_POSTGRESQL_PARTITION_POLL_INTERVAL`

No documentation available.

### `consumers[].postgresql.partition_notification_channel`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_POSTGRESQL_PARTITION_NOTIFICATION_CHANNEL`

No documentation available.

### `consumers[].postgresql.tls`

Type: `TLSConfig` object

No documentation available.

#### `consumers[].postgresql.tls.enabled`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_POSTGRESQL_TLS_ENABLED`

`enabled` turns on using TLS.

#### `consumers[].postgresql.tls.cert_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_POSTGRESQL_TLS_CERT_PEM`

`cert_pem` is a PEM certificate.

#### `consumers[].postgresql.tls.key_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_POSTGRESQL_TLS_KEY_PEM`

`key_pem` is a path to a file with key in PEM format.

#### `consumers[].postgresql.tls.server_ca_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_POSTGRESQL_TLS_SERVER_CA_PEM`

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

#### `consumers[].postgresql.tls.client_ca_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_POSTGRESQL_TLS_CLIENT_CA_PEM`

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

#### `consumers[].postgresql.tls.insecure_skip_verify`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_POSTGRESQL_TLS_INSECURE_SKIP_VERIFY`

`insecure_skip_verify` turns off server certificate verification.

#### `consumers[].postgresql.tls.server_name`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_POSTGRESQL_TLS_SERVER_NAME`

`server_name` is used to verify the hostname on the returned certificates.

## `consumers[].kafka`

Type: `KafkaConsumerConfig` object

`kafka` allows defining options for consumer of kafka type.

### `consumers[].kafka.brokers`

Type: `[]string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_BROKERS`

No documentation available.

### `consumers[].kafka.topics`

Type: `[]string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_TOPICS`

No documentation available.

### `consumers[].kafka.consumer_group`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_CONSUMER_GROUP`

No documentation available.

### `consumers[].kafka.max_poll_records`

Type: `int`. Default: `100`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_MAX_POLL_RECORDS`

No documentation available.

### `consumers[].kafka.tls`

Type: `TLSConfig` object

`tls` for the connection to Kafka.

#### `consumers[].kafka.tls.enabled`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_TLS_ENABLED`

`enabled` turns on using TLS.

#### `consumers[].kafka.tls.cert_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_TLS_CERT_PEM`

`cert_pem` is a PEM certificate.

#### `consumers[].kafka.tls.key_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_TLS_KEY_PEM`

`key_pem` is a path to a file with key in PEM format.

#### `consumers[].kafka.tls.server_ca_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_TLS_SERVER_CA_PEM`

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

#### `consumers[].kafka.tls.client_ca_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_TLS_CLIENT_CA_PEM`

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

#### `consumers[].kafka.tls.insecure_skip_verify`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_TLS_INSECURE_SKIP_VERIFY`

`insecure_skip_verify` turns off server certificate verification.

#### `consumers[].kafka.tls.server_name`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_TLS_SERVER_NAME`

`server_name` is used to verify the hostname on the returned certificates.

### `consumers[].kafka.sasl_mechanism`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_SASL_MECHANISM`

`sasl_mechanism` when not empty enables SASL auth.

### `consumers[].kafka.sasl_user`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_SASL_USER`

No documentation available.

### `consumers[].kafka.sasl_password`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_SASL_PASSWORD`

No documentation available.

### `consumers[].kafka.partition_buffer_size`

Type: `int`. Default: `16`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_PARTITION_BUFFER_SIZE`

`partition_buffer_size` is the size of the buffer for each partition consumer.
This is the number of records that can be buffered before the consumer
will pause fetching records from Kafka. By default, this is 16.

### `consumers[].kafka.fetch_max_bytes`

Type: `int32`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_FETCH_MAX_BYTES`

`fetch_max_bytes` is the maximum number of bytes to fetch from Kafka in a single request.
If not set the default 50MB is used.

### `consumers[].kafka.method_header`

Type: `string`. Default: `centrifugo-method`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_METHOD_HEADER`

`method_header` is a header name to extract method name from Kafka message.
If provided in message, then payload must be just a serialized API request object.

### `consumers[].kafka.publication_data_mode`

Type: `KafkaPublicationDataModeConfig` object

`publication_data_mode` is a configuration for the mode where message payload already
contains data ready to publish into channels, instead of API command.

#### `consumers[].kafka.publication_data_mode.enabled`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_PUBLICATION_DATA_MODE_ENABLED`

`enabled` enables Kafka publication data mode for the Kafka consumer.

#### `consumers[].kafka.publication_data_mode.channels_header`

Type: `string`. Default: `centrifugo-channels`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_PUBLICATION_DATA_MODE_CHANNELS_HEADER`

`channels_header` is a header name to extract channels to publish data into
(channels must be comma-separated). Ex. of value: "channel1,channel2".

#### `consumers[].kafka.publication_data_mode.idempotency_key_header`

Type: `string`. Default: `centrifugo-idempotency-key`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_PUBLICATION_DATA_MODE_IDEMPOTENCY_KEY_HEADER`

`idempotency_key_header` is a header name to extract Publication idempotency key from
Kafka message. See https://centrifugal.dev/docs/server/server_api#publishrequest.

#### `consumers[].kafka.publication_data_mode.delta_header`

Type: `string`. Default: `centrifugo-delta`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_PUBLICATION_DATA_MODE_DELTA_HEADER`

`delta_header` is a header name to extract Publication delta flag from Kafka message
which tells Centrifugo whether to use delta compression for message or not.
See https://centrifugal.dev/docs/server/delta_compression and
https://centrifugal.dev/docs/server/server_api#publishrequest.

#### `consumers[].kafka.publication_data_mode.version_header`

Type: `string`. Default: `centrifugo-version`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_PUBLICATION_DATA_MODE_VERSION_HEADER`

`version_header` is a header name to extract Publication version from Kafka message.

#### `consumers[].kafka.publication_data_mode.version_epoch_header`

Type: `string`. Default: `centrifugo-version-epoch`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_PUBLICATION_DATA_MODE_VERSION_EPOCH_HEADER`

`version_epoch_header` is a header name to extract Publication version epoch from Kafka message.

#### `consumers[].kafka.publication_data_mode.tags_header_prefix`

Type: `string`. Default: `centrifugo-tag-`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_KAFKA_PUBLICATION_DATA_MODE_TAGS_HEADER_PREFIX`

`tags_header_prefix` is a prefix for headers that contain tags to attach to Publication.

## `consumers[].nats_jetstream`

Type: `NatsJetStreamConsumerConfig` object

`nats_jetstream` allows defining options for consumer of nats_jetstream type.

### `consumers[].nats_jetstream.url`

Type: `string`. Default: `nats://127.0.0.1:4222`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_URL`

`url` is the address of the NATS server.

### `consumers[].nats_jetstream.credentials_file`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_CREDENTIALS_FILE`

`credentials_file` is the path to a NATS credentials file used for authentication (nats.UserCredentials).
If provided, it overrides username/password and token.

### `consumers[].nats_jetstream.username`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_USERNAME`

`username` is used for basic authentication (along with Password) if CredentialsFile is not provided.

### `consumers[].nats_jetstream.password`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_PASSWORD`

`password` is used with Username for basic authentication.

### `consumers[].nats_jetstream.token`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_TOKEN`

`token` is an alternative authentication mechanism if CredentialsFile and Username are not provided.

### `consumers[].nats_jetstream.stream_name`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_STREAM_NAME`

`stream_name` is the name of the NATS JetStream stream to use.

### `consumers[].nats_jetstream.subjects`

Type: `[]string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_SUBJECTS`

`subjects` is the list of NATS subjects (topics) to filter.

### `consumers[].nats_jetstream.durable_consumer_name`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_DURABLE_CONSUMER_NAME`

`durable_consumer_name` sets the name of the durable JetStream consumer to use.

### `consumers[].nats_jetstream.deliver_policy`

Type: `string`. Default: `new`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_DELIVER_POLICY`

`deliver_policy` is the NATS JetStream delivery policy for the consumer. By default, it is set to "new". Possible values: `new`, `all`.

### `consumers[].nats_jetstream.max_ack_pending`

Type: `int`. Default: `100`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_MAX_ACK_PENDING`

`max_ack_pending` is the maximum number of unacknowledged messages that can be pending for the consumer.

### `consumers[].nats_jetstream.method_header`

Type: `string`. Default: `centrifugo-method`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_METHOD_HEADER`

`method_header` is the NATS message header used to extract the method name for dispatching commands.
If provided in message, then payload must be just a serialized API request object.

### `consumers[].nats_jetstream.publication_data_mode`

Type: `NatsJetStreamPublicationDataModeConfig` object

`publication_data_mode` configures extraction of pre-formatted publication data from message headers.

#### `consumers[].nats_jetstream.publication_data_mode.enabled`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_PUBLICATION_DATA_MODE_ENABLED`

`enabled` toggles publication data mode.

#### `consumers[].nats_jetstream.publication_data_mode.channels_header`

Type: `string`. Default: `centrifugo-channels`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_PUBLICATION_DATA_MODE_CHANNELS_HEADER`

`channels_header` is the name of the header that contains comma-separated channel names.

#### `consumers[].nats_jetstream.publication_data_mode.idempotency_key_header`

Type: `string`. Default: `centrifugo-idempotency-key`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_PUBLICATION_DATA_MODE_IDEMPOTENCY_KEY_HEADER`

`idempotency_key_header` is the name of the header that contains an idempotency key for deduplication.

#### `consumers[].nats_jetstream.publication_data_mode.delta_header`

Type: `string`. Default: `centrifugo-delta`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_PUBLICATION_DATA_MODE_DELTA_HEADER`

`delta_header` is the name of the header indicating whether the message represents a delta (partial update).

#### `consumers[].nats_jetstream.publication_data_mode.version_header`

Type: `string`. Default: `centrifugo-version`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_PUBLICATION_DATA_MODE_VERSION_HEADER`

`version_header` is the name of the header that contains the version of the message.

#### `consumers[].nats_jetstream.publication_data_mode.version_epoch_header`

Type: `string`. Default: `centrifugo-version-epoch`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_PUBLICATION_DATA_MODE_VERSION_EPOCH_HEADER`

`version_epoch_header` is the name of the header that contains the version epoch of the message.

#### `consumers[].nats_jetstream.publication_data_mode.tags_header_prefix`

Type: `string`. Default: `centrifugo-tag-`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_PUBLICATION_DATA_MODE_TAGS_HEADER_PREFIX`

`tags_header_prefix` is the prefix used to extract dynamic tags from message headers.

### `consumers[].nats_jetstream.tls`

Type: `TLSConfig` object

`tls` is the configuration for TLS.

#### `consumers[].nats_jetstream.tls.enabled`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_TLS_ENABLED`

`enabled` turns on using TLS.

#### `consumers[].nats_jetstream.tls.cert_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_TLS_CERT_PEM`

`cert_pem` is a PEM certificate.

#### `consumers[].nats_jetstream.tls.key_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_TLS_KEY_PEM`

`key_pem` is a path to a file with key in PEM format.

#### `consumers[].nats_jetstream.tls.server_ca_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_TLS_SERVER_CA_PEM`

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

#### `consumers[].nats_jetstream.tls.client_ca_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_TLS_CLIENT_CA_PEM`

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

#### `consumers[].nats_jetstream.tls.insecure_skip_verify`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_TLS_INSECURE_SKIP_VERIFY`

`insecure_skip_verify` turns off server certificate verification.

#### `consumers[].nats_jetstream.tls.server_name`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_NATS_JETSTREAM_TLS_SERVER_NAME`

`server_name` is used to verify the hostname on the returned certificates.

## `consumers[].redis_stream`

Type: `RedisStreamConsumerConfig` object

`redis_stream` allows defining options for consumer of redis_stream type.

### `consumers[].redis_stream.address`

Type: `[]string`. Default: `redis://127.0.0.1:6379`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_ADDRESS`

`address` is a list of Redis shard addresses. In most cases a single shard is used. But when many
addresses provided Centrifugo will distribute keys between shards using consistent hashing.

### `consumers[].redis_stream.prefix`

Type: `string`. Default: `centrifugo`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_PREFIX`

`prefix` for all Redis keys and channels.

### `consumers[].redis_stream.connect_timeout`

Type: `Duration`. Default: `1s`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_CONNECT_TIMEOUT`

`connect_timeout` is a timeout for establishing connection to Redis.

### `consumers[].redis_stream.io_timeout`

Type: `Duration`. Default: `4s`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_IO_TIMEOUT`

`io_timeout` is a timeout for all read/write operations against Redis (can be considered as a request timeout).

### `consumers[].redis_stream.db`

Type: `int`. Default: `0`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_DB`

`db` is a Redis database to use. Generally it's not recommended to use non-zero DB. Note, that Redis
PUB/SUB is global for all databases in a single Redis instance. So when using non-zero DB make sure
that different Centrifugo setups use different prefixes.

### `consumers[].redis_stream.user`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_USER`

`user` is a Redis user.

### `consumers[].redis_stream.password`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_PASSWORD`

`password` is a Redis password.

### `consumers[].redis_stream.client_name`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_CLIENT_NAME`

`client_name` allows changing a Redis client name used when connecting.

### `consumers[].redis_stream.force_resp2`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_FORCE_RESP2`

`force_resp2` forces use of Redis Resp2 protocol for communication.

### `consumers[].redis_stream.cluster_address`

Type: `[]string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_CLUSTER_ADDRESS`

`cluster_address` is a list of Redis cluster addresses. When several provided - data will be sharded
between them using consistent hashing. Several Cluster addresses within one shard may be passed
comma-separated.

### `consumers[].redis_stream.tls`

Type: `TLSConfig` object

`tls` is a configuration for Redis TLS support.

#### `consumers[].redis_stream.tls.enabled`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_TLS_ENABLED`

`enabled` turns on using TLS.

#### `consumers[].redis_stream.tls.cert_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_TLS_CERT_PEM`

`cert_pem` is a PEM certificate.

#### `consumers[].redis_stream.tls.key_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_TLS_KEY_PEM`

`key_pem` is a path to a file with key in PEM format.

#### `consumers[].redis_stream.tls.server_ca_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_TLS_SERVER_CA_PEM`

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

#### `consumers[].redis_stream.tls.client_ca_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_TLS_CLIENT_CA_PEM`

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

#### `consumers[].redis_stream.tls.insecure_skip_verify`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_TLS_INSECURE_SKIP_VERIFY`

`insecure_skip_verify` turns off server certificate verification.

#### `consumers[].redis_stream.tls.server_name`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_TLS_SERVER_NAME`

`server_name` is used to verify the hostname on the returned certificates.

### `consumers[].redis_stream.sentinel_address`

Type: `[]string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_SENTINEL_ADDRESS`

`sentinel_address` allows setting Redis Sentinel addresses. When provided - Sentinel will be used.
When multiple addresses provided - data will be sharded between them using consistent hashing.
Several Sentinel addresses within one shard may be passed comma-separated.

### `consumers[].redis_stream.sentinel_user`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_SENTINEL_USER`

`sentinel_user` is a Redis Sentinel user.

### `consumers[].redis_stream.sentinel_password`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_SENTINEL_PASSWORD`

`sentinel_password` is a Redis Sentinel password.

### `consumers[].redis_stream.sentinel_master_name`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_SENTINEL_MASTER_NAME`

`sentinel_master_name` is a Redis master name in Sentinel setup.

### `consumers[].redis_stream.sentinel_client_name`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_SENTINEL_CLIENT_NAME`

`sentinel_client_name` is a Redis Sentinel client name used when connecting.

### `consumers[].redis_stream.sentinel_tls`

Type: `TLSConfig` object

`sentinel_tls` is a configuration for Redis Sentinel TLS support.

#### `consumers[].redis_stream.sentinel_tls.enabled`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_SENTINEL_TLS_ENABLED`

`enabled` turns on using TLS.

#### `consumers[].redis_stream.sentinel_tls.cert_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_SENTINEL_TLS_CERT_PEM`

`cert_pem` is a PEM certificate.

#### `consumers[].redis_stream.sentinel_tls.key_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_SENTINEL_TLS_KEY_PEM`

`key_pem` is a path to a file with key in PEM format.

#### `consumers[].redis_stream.sentinel_tls.server_ca_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_SENTINEL_TLS_SERVER_CA_PEM`

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

#### `consumers[].redis_stream.sentinel_tls.client_ca_pem`

Type: `PEMData`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_SENTINEL_TLS_CLIENT_CA_PEM`

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

#### `consumers[].redis_stream.sentinel_tls.insecure_skip_verify`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_SENTINEL_TLS_INSECURE_SKIP_VERIFY`

`insecure_skip_verify` turns off server certificate verification.

#### `consumers[].redis_stream.sentinel_tls.server_name`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_SENTINEL_TLS_SERVER_NAME`

`server_name` is used to verify the hostname on the returned certificates.

### `consumers[].redis_stream.replica_client`

Type: `RedisReplicaClient` object

`replica_client` is a configuration fot Redis replica client.

#### `consumers[].redis_stream.replica_client.enabled`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_REPLICA_CLIENT_ENABLED`

`enabled` enables replica client.

### `consumers[].redis_stream.streams`

Type: `[]string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_STREAMS`

`streams` to consume.

### `consumers[].redis_stream.consumer_group`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_CONSUMER_GROUP`

`consumer_group` name to use.

### `consumers[].redis_stream.visibility_timeout`

Type: `Duration`. Default: `30s`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_VISIBILITY_TIMEOUT`

`visibility_timeout` is the time to wait for a message to be processed before it is re-queued.

### `consumers[].redis_stream.num_workers`

Type: `int`. Default: `1`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_NUM_WORKERS`

`num_workers` is the number of message workers to use for processing for each stream.

### `consumers[].redis_stream.payload_value`

Type: `string`. Default: `payload`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_PAYLOAD_VALUE`

`payload_value` is used to extract data from Redis Stream message.

### `consumers[].redis_stream.method_value`

Type: `string`. Default: `method`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_METHOD_VALUE`

`method_value` is used to extract a method for command messages.
If provided in message, then payload must be just a serialized API request object.

### `consumers[].redis_stream.publication_data_mode`

Type: `RedisStreamPublicationDataModeConfig` object

`publication_data_mode` configures publication data mode.

#### `consumers[].redis_stream.publication_data_mode.enabled`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_PUBLICATION_DATA_MODE_ENABLED`

`enabled` toggles publication data mode.

#### `consumers[].redis_stream.publication_data_mode.channels_value`

Type: `string`. Default: `centrifugo-channels`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_PUBLICATION_DATA_MODE_CHANNELS_VALUE`

`channels_value` is used to extract channels to publish data into (channels must be comma-separated).

#### `consumers[].redis_stream.publication_data_mode.idempotency_key_value`

Type: `string`. Default: `centrifugo-idempotency-key`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_PUBLICATION_DATA_MODE_IDEMPOTENCY_KEY_VALUE`

`idempotency_key_value` is used to extract Publication idempotency key from Redis Stream message.

#### `consumers[].redis_stream.publication_data_mode.delta_value`

Type: `string`. Default: `centrifugo-delta`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_PUBLICATION_DATA_MODE_DELTA_VALUE`

`delta_value` is used to extract Publication delta flag from Redis Stream message.

#### `consumers[].redis_stream.publication_data_mode.version_value`

Type: `string`. Default: `centrifugo-version`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_PUBLICATION_DATA_MODE_VERSION_VALUE`

`version_value` is used to extract Publication version from Redis Stream message.

#### `consumers[].redis_stream.publication_data_mode.version_epoch_value`

Type: `string`. Default: `centrifugo-version-epoch`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_PUBLICATION_DATA_MODE_VERSION_EPOCH_VALUE`

`version_epoch_value` is used to extract Publication version epoch from Redis Stream message.

#### `consumers[].redis_stream.publication_data_mode.tags_value_prefix`

Type: `string`. Default: `centrifugo-tag-`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_REDIS_STREAM_PUBLICATION_DATA_MODE_TAGS_VALUE_PREFIX`

`tags_value_prefix` is used to extract Publication tags from Redis Stream message.

## `consumers[].google_pub_sub`

Type: `GooglePubSubConsumerConfig` object

`google_pub_sub` allows defining options for consumer of google_pub_sub type.

### `consumers[].google_pub_sub.project_id`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_PROJECT_ID`

Google Cloud project ID.

### `consumers[].google_pub_sub.subscriptions`

Type: `[]string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_SUBSCRIPTIONS`

`subscriptions` is the list of Pub/Sub subscription ids to consume from.

### `consumers[].google_pub_sub.max_outstanding_messages`

Type: `int`. Default: `100`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_MAX_OUTSTANDING_MESSAGES`

`max_outstanding_messages` controls the maximum number of unprocessed messages.

### `consumers[].google_pub_sub.max_outstanding_bytes`

Type: `int`. Default: `1000000`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_MAX_OUTSTANDING_BYTES`

`max_outstanding_bytes` controls the maximum number of unprocessed bytes.

### `consumers[].google_pub_sub.auth_mechanism`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_AUTH_MECHANISM`

`auth_mechanism` specifies which authentication mechanism to use:
"default", "service_account".

### `consumers[].google_pub_sub.credentials_file`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_CREDENTIALS_FILE`

`credentials_file` is the path to the service account JSON file if required.

### `consumers[].google_pub_sub.method_attribute`

Type: `string`. Default: `centrifugo-method`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_METHOD_ATTRIBUTE`

`method_attribute` is an attribute name to extract a method name from the message.
If provided in message, then payload must be just a serialized API request object.

### `consumers[].google_pub_sub.publication_data_mode`

Type: `GooglePubSubPublicationDataModeConfig` object

`publication_data_mode` holds settings for the mode where message payload already contains data
ready to publish into channels.

#### `consumers[].google_pub_sub.publication_data_mode.enabled`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_PUBLICATION_DATA_MODE_ENABLED`

`enabled` enables publication data mode.

#### `consumers[].google_pub_sub.publication_data_mode.channels_attribute`

Type: `string`. Default: `centrifugo-channels`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_PUBLICATION_DATA_MODE_CHANNELS_ATTRIBUTE`

`channels_attribute` is the attribute name containing comma-separated channel names.

#### `consumers[].google_pub_sub.publication_data_mode.idempotency_key_attribute`

Type: `string`. Default: `centrifugo-idempotency-key`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_PUBLICATION_DATA_MODE_IDEMPOTENCY_KEY_ATTRIBUTE`

`idempotency_key_attribute` is the attribute name for an idempotency key.

#### `consumers[].google_pub_sub.publication_data_mode.delta_attribute`

Type: `string`. Default: `centrifugo-delta`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_PUBLICATION_DATA_MODE_DELTA_ATTRIBUTE`

`delta_attribute` is the attribute name for a delta flag.

#### `consumers[].google_pub_sub.publication_data_mode.version_attribute`

Type: `string`. Default: `centrifugo-version`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_PUBLICATION_DATA_MODE_VERSION_ATTRIBUTE`

`version_attribute` is the attribute name for a version.

#### `consumers[].google_pub_sub.publication_data_mode.version_epoch_attribute`

Type: `string`. Default: `centrifugo-version-epoch`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_PUBLICATION_DATA_MODE_VERSION_EPOCH_ATTRIBUTE`

`version_epoch_attribute` is the attribute name for a version epoch.

#### `consumers[].google_pub_sub.publication_data_mode.tags_attribute_prefix`

Type: `string`. Default: `centrifugo-tag-`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_GOOGLE_PUB_SUB_PUBLICATION_DATA_MODE_TAGS_ATTRIBUTE_PREFIX`

`tags_attribute_prefix` is the prefix for attributes containing tags.

## `consumers[].aws_sqs`

Type: `AwsSqsConsumerConfig` object

`aws_sqs` allows defining options for consumer of aws_sqs type.

### `consumers[].aws_sqs.queues`

Type: `[]string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_QUEUES`

`queues` is a list of SQS queue URLs to consume.

### `consumers[].aws_sqs.sns_envelope`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_SNS_ENVELOPE`

`sns_envelope`, when true, expects messages to be wrapped in an SNS envelope – this is required when
consuming from SNS topics with SQS subscriptions.

### `consumers[].aws_sqs.region`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_REGION`

`region` is the AWS region.

### `consumers[].aws_sqs.max_number_of_messages`

Type: `int32`. Default: `10`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_MAX_NUMBER_OF_MESSAGES`

`max_number_of_messages` is the maximum number of messages to receive per poll.

### `consumers[].aws_sqs.wait_time_time`

Type: `Duration`. Default: `20s`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_WAIT_TIME_TIME`

`wait_time_time` is the long-poll wait time. Rounded to seconds internally.

### `consumers[].aws_sqs.visibility_timeout`

Type: `Duration`. Default: `30s`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_VISIBILITY_TIMEOUT`

`visibility_timeout` is the time a message is hidden from other consumers. Rounded to seconds internally.

### `consumers[].aws_sqs.max_concurrency`

Type: `int`. Default: `1`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_MAX_CONCURRENCY`

`max_concurrency` defines max concurrency during message batch processing.

### `consumers[].aws_sqs.credentials_profile`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_CREDENTIALS_PROFILE`

`credentials_profile` is a shared credentials profile to use.

### `consumers[].aws_sqs.assume_role_arn`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_ASSUME_ROLE_ARN`

`assume_role_arn`, if provided, will cause the consumer to assume the given IAM role.

### `consumers[].aws_sqs.method_attribute`

Type: `string`. Default: `centrifugo-method`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_METHOD_ATTRIBUTE`

`method_attribute` is the attribute name to extract a method for command messages.
If provided in message, then payload must be just a serialized API request object.

### `consumers[].aws_sqs.localstack_endpoint`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_LOCALSTACK_ENDPOINT`

`localstack_endpoint` if set enables using localstack with provided URL.

### `consumers[].aws_sqs.publication_data_mode`

Type: `AWSPublicationDataModeConfig` object

`publication_data_mode` holds settings for the mode where message payload already contains data
ready to publish into channels.

#### `consumers[].aws_sqs.publication_data_mode.enabled`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_PUBLICATION_DATA_MODE_ENABLED`

`enabled` enables publication data mode.

#### `consumers[].aws_sqs.publication_data_mode.channels_attribute`

Type: `string`. Default: `centrifugo-channels`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_PUBLICATION_DATA_MODE_CHANNELS_ATTRIBUTE`

`channels_attribute` is the attribute name containing comma-separated channel names.

#### `consumers[].aws_sqs.publication_data_mode.idempotency_key_attribute`

Type: `string`. Default: `centrifugo-idempotency-key`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_PUBLICATION_DATA_MODE_IDEMPOTENCY_KEY_ATTRIBUTE`

`idempotency_key_attribute` is the attribute name for an idempotency key.

#### `consumers[].aws_sqs.publication_data_mode.delta_attribute`

Type: `string`. Default: `centrifugo-delta`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_PUBLICATION_DATA_MODE_DELTA_ATTRIBUTE`

`delta_attribute` is the attribute name for a delta flag.

#### `consumers[].aws_sqs.publication_data_mode.version_attribute`

Type: `string`. Default: `centrifugo-version`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_PUBLICATION_DATA_MODE_VERSION_ATTRIBUTE`

`version_attribute` is the attribute name for a version of publication.

#### `consumers[].aws_sqs.publication_data_mode.version_epoch_attribute`

Type: `string`. Default: `centrifugo-version-epoch`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_PUBLICATION_DATA_MODE_VERSION_EPOCH_ATTRIBUTE`

`version_epoch_attribute` is the attribute name for a version epoch of publication.

#### `consumers[].aws_sqs.publication_data_mode.tags_attribute_prefix`

Type: `string`. Default: `centrifugo-tag-`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AWS_SQS_PUBLICATION_DATA_MODE_TAGS_ATTRIBUTE_PREFIX`

`tags_attribute_prefix` is the prefix for attributes containing tags.

## `consumers[].azure_service_bus`

Type: `AzureServiceBusConsumerConfig` object

`azure_service_bus` allows defining options for consumer of azure_service_bus type.

### `consumers[].azure_service_bus.connection_string`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_CONNECTION_STRING`

`connection_string` is the full connection string used for connection-string–based authentication.

### `consumers[].azure_service_bus.use_azure_identity`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_USE_AZURE_IDENTITY`

`use_azure_identity` toggles Azure Identity (AAD) authentication instead of connection strings.

### `consumers[].azure_service_bus.fully_qualified_namespace`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_FULLY_QUALIFIED_NAMESPACE`

`fully_qualified_namespace` is the Service Bus namespace, e.g. "your-namespace.servicebus.windows.net".

### `consumers[].azure_service_bus.tenant_id`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_TENANT_ID`

`tenant_id` is the Azure Active Directory tenant ID used with Azure Identity.

### `consumers[].azure_service_bus.client_id`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_CLIENT_ID`

`client_id` is the Azure AD application (client) ID used for authentication.

### `consumers[].azure_service_bus.client_secret`

Type: `string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_CLIENT_SECRET`

`client_secret` is the secret associated with the Azure AD application.

### `consumers[].azure_service_bus.queues`

Type: `[]string`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_QUEUES`

`queues` is the list of the Azure Service Bus queues to consume from.

### `consumers[].azure_service_bus.use_sessions`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_USE_SESSIONS`

`use_sessions` enables session-aware message handling.
All messages must include a SessionID; messages within the same session will be processed in order.

### `consumers[].azure_service_bus.max_concurrent_calls`

Type: `int`. Default: `1`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_MAX_CONCURRENT_CALLS`

`max_concurrent_calls` controls the maximum number of messages processed concurrently.

### `consumers[].azure_service_bus.max_receive_messages`

Type: `int`. Default: `1`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_MAX_RECEIVE_MESSAGES`

`max_receive_messages` sets the batch size when receiving messages from the queue.

### `consumers[].azure_service_bus.method_property`

Type: `string`. Default: `centrifugo-method`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_METHOD_PROPERTY`

`method_property` is the name of the message property used to extract the method (for API command).
If provided in message, then payload must be just a serialized API request object.

### `consumers[].azure_service_bus.publication_data_mode`

Type: `AzureServiceBusPublicationDataModeConfig` object

`publication_data_mode` configures how structured publication-ready data is extracted from the message.

#### `consumers[].azure_service_bus.publication_data_mode.enabled`

Type: `bool`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_PUBLICATION_DATA_MODE_ENABLED`

`enabled` toggles the publication data mode.

#### `consumers[].azure_service_bus.publication_data_mode.channels_property`

Type: `string`. Default: `centrifugo-channels`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_PUBLICATION_DATA_MODE_CHANNELS_PROPERTY`

`channels_property` is the name of the message property that contains the list of target channels.

#### `consumers[].azure_service_bus.publication_data_mode.idempotency_key_property`

Type: `string`. Default: `centrifugo-idempotency-key`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_PUBLICATION_DATA_MODE_IDEMPOTENCY_KEY_PROPERTY`

`idempotency_key_property` is the property that holds an idempotency key for deduplication.

#### `consumers[].azure_service_bus.publication_data_mode.delta_property`

Type: `string`. Default: `centrifugo-delta`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_PUBLICATION_DATA_MODE_DELTA_PROPERTY`

`delta_property` is the property that represents changes or deltas in the payload.

#### `consumers[].azure_service_bus.publication_data_mode.version_property`

Type: `string`. Default: `centrifugo-version`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_PUBLICATION_DATA_MODE_VERSION_PROPERTY`

`version_property` is the property that holds the version of the message.

#### `consumers[].azure_service_bus.publication_data_mode.version_epoch_property`

Type: `string`. Default: `centrifugo-version-epoch`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_PUBLICATION_DATA_MODE_VERSION_EPOCH_PROPERTY`

`version_epoch_property` is the property that holds the version epoch of the message.

#### `consumers[].azure_service_bus.publication_data_mode.tags_property_prefix`

Type: `string`. Default: `centrifugo-tag-`

Env: `CENTRIFUGO_CONSUMERS_<NAME>_AZURE_SERVICE_BUS_PUBLICATION_DATA_MODE_TAGS_PROPERTY_PREFIX`

`tags_property_prefix` defines the prefix used to extract dynamic tags from message properties.


