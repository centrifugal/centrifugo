# Centrifugo configuration options

Config contains configuration options of Centrifugo.

## `http_server`

Type: `HTTPServer` object.

`http_server` is a configuration for Centrifugo HTTP server.

### `http_server.address`

Type: `string`.

`address` to bind HTTP server to.

### `http_server.port`

Type: `int`. Default: `8000`.

`port` to bind HTTP server to.

### `http_server.internal_address`

Type: `string`.

`internal_address` to bind internal HTTP server to. Internal server is used to serve endpoints
which are normally should not be exposed to the outside world.

### `http_server.internal_port`

Type: `string`.

`internal_port` to bind internal HTTP server to.

### `http_server.tls`

Type: `TLSConfig` object.

`tls` configuration for HTTP server.

#### `http_server.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

#### `http_server.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

#### `http_server.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

#### `http_server.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

#### `http_server.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

#### `http_server.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

#### `http_server.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

### `http_server.tls_autocert`

Type: `TLSAutocert` object.

`tls_autocert` for automatic TLS certificates from ACME provider (ex. Let's Encrypt).

#### `http_server.tls_autocert.enabled`

Type: `bool`.

No documentation available.

#### `http_server.tls_autocert.host_whitelist`

Type: `[]string`.

No documentation available.

#### `http_server.tls_autocert.cache_dir`

Type: `string`.

No documentation available.

#### `http_server.tls_autocert.email`

Type: `string`.

No documentation available.

#### `http_server.tls_autocert.server_name`

Type: `string`.

No documentation available.

#### `http_server.tls_autocert.http`

Type: `bool`.

No documentation available.

#### `http_server.tls_autocert.http_addr`

Type: `string`. Default: `:80`.

No documentation available.

### `http_server.tls_external`

Type: `bool`.

`tls_external` enables TLS only for external HTTP endpoints.

### `http_server.internal_tls`

Type: `TLSConfig` object.

`internal_tls` is a custom configuration for internal HTTP endpoints. If not set InternalTLS will be the same as TLS.

#### `http_server.internal_tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

#### `http_server.internal_tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

#### `http_server.internal_tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

#### `http_server.internal_tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

#### `http_server.internal_tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

#### `http_server.internal_tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

#### `http_server.internal_tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

### `http_server.http3`

Type: `HTTP3` object.

`http3` allows enabling HTTP/3 support. EXPERIMENTAL!

#### `http_server.http3.enabled`

Type: `bool`.

No documentation available.

## `log`

Type: `Log` object.

`log` is a configuration for logging.

### `log.level`

Type: `string`. Default: `info`.

`level` is a log level for Centrifugo logger. Supported values: none, trace, debug, info, warn, error.

### `log.file`

Type: `string`.

`file` is a path to log file. If not set logs go to stdout.

## `engine`

Type: `Engine` object.

`engine` is a configuration for Centrifugo engine. It's a handy combination of Broker and PresenceManager.
Currently only memory and redis engines are supported – both implement all the features. For more granular
control use Broker and PresenceManager options.

### `engine.type`

Type: `string`. Default: `memory`.

`type` of broker to use. Can be "memory" or "redis" at this point.

### `engine.redis`

Type: `RedisEngine` object.

`redis` is a configuration for "redis" broker.

#### `engine.redis.address`

Type: `[]string`. Default: `redis://127.0.0.1:6379`.

No documentation available.

#### `engine.redis.prefix`

Type: `string`. Default: `centrifugo`.

No documentation available.

#### `engine.redis.connect_timeout`

Type: `Duration`. Default: `1s`.

No documentation available.

#### `engine.redis.io_timeout`

Type: `Duration`. Default: `4s`.

No documentation available.

#### `engine.redis.db`

Type: `int`. Default: `0`.

No documentation available.

#### `engine.redis.user`

Type: `string`.

No documentation available.

#### `engine.redis.password`

Type: `string`.

No documentation available.

#### `engine.redis.client_name`

Type: `string`.

No documentation available.

#### `engine.redis.force_resp2`

Type: `bool`.

No documentation available.

#### `engine.redis.cluster_address`

Type: `[]string`.

No documentation available.

#### `engine.redis.tls`

Type: `TLSConfig` object.

No documentation available.

##### `engine.redis.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `engine.redis.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `engine.redis.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `engine.redis.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `engine.redis.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `engine.redis.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `engine.redis.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

#### `engine.redis.sentinel_address`

Type: `[]string`.

No documentation available.

#### `engine.redis.sentinel_user`

Type: `string`.

No documentation available.

#### `engine.redis.sentinel_password`

Type: `string`.

No documentation available.

#### `engine.redis.sentinel_master_name`

Type: `string`.

No documentation available.

#### `engine.redis.sentinel_client_name`

Type: `string`.

No documentation available.

#### `engine.redis.sentinel_tls`

Type: `TLSConfig` object.

No documentation available.

##### `engine.redis.sentinel_tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `engine.redis.sentinel_tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `engine.redis.sentinel_tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `engine.redis.sentinel_tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `engine.redis.sentinel_tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `engine.redis.sentinel_tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `engine.redis.sentinel_tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

#### `engine.redis.history_use_lists`

Type: `bool`.

No documentation available.

#### `engine.redis.presence_ttl`

Type: `Duration`. Default: `60s`.

No documentation available.

#### `engine.redis.presence_hash_field_ttl`

Type: `bool`.

No documentation available.

#### `engine.redis.presence_user_mapping`

Type: `bool`.

No documentation available.

## `broker`

Type: `Broker` object.

`broker` allows to configure a message broker to use. Broker is responsible for PUB/SUB functionality
and channel message history and idempotency cache .
By default, memory Broker is used. Memory broker is superfast, but it's not distributed and all
data stored in memory (thus lost after node restart). Redis Broker provides seamless horizontal
scalability, fault-tolerance, and persistence over Centrifugo restarts. Centrifugo also supports
Nats Broker which only implements at most once PUB/SUB semantics.

### `broker.enabled`

Type: `bool`.

No documentation available.

### `broker.type`

Type: `string`. Default: `memory`.

`type` of broker to use. Can be "memory", "redis", "nats" at this point.

### `broker.redis`

Type: `RedisBroker` object.

`redis` is a configuration for "redis" broker.

#### `broker.redis.address`

Type: `[]string`. Default: `redis://127.0.0.1:6379`.

No documentation available.

#### `broker.redis.prefix`

Type: `string`. Default: `centrifugo`.

No documentation available.

#### `broker.redis.connect_timeout`

Type: `Duration`. Default: `1s`.

No documentation available.

#### `broker.redis.io_timeout`

Type: `Duration`. Default: `4s`.

No documentation available.

#### `broker.redis.db`

Type: `int`. Default: `0`.

No documentation available.

#### `broker.redis.user`

Type: `string`.

No documentation available.

#### `broker.redis.password`

Type: `string`.

No documentation available.

#### `broker.redis.client_name`

Type: `string`.

No documentation available.

#### `broker.redis.force_resp2`

Type: `bool`.

No documentation available.

#### `broker.redis.cluster_address`

Type: `[]string`.

No documentation available.

#### `broker.redis.tls`

Type: `TLSConfig` object.

No documentation available.

##### `broker.redis.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `broker.redis.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `broker.redis.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `broker.redis.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `broker.redis.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `broker.redis.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `broker.redis.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

#### `broker.redis.sentinel_address`

Type: `[]string`.

No documentation available.

#### `broker.redis.sentinel_user`

Type: `string`.

No documentation available.

#### `broker.redis.sentinel_password`

Type: `string`.

No documentation available.

#### `broker.redis.sentinel_master_name`

Type: `string`.

No documentation available.

#### `broker.redis.sentinel_client_name`

Type: `string`.

No documentation available.

#### `broker.redis.sentinel_tls`

Type: `TLSConfig` object.

No documentation available.

##### `broker.redis.sentinel_tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `broker.redis.sentinel_tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `broker.redis.sentinel_tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `broker.redis.sentinel_tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `broker.redis.sentinel_tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `broker.redis.sentinel_tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `broker.redis.sentinel_tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

#### `broker.redis.history_use_lists`

Type: `bool`.

No documentation available.

### `broker.nats`

Type: `NatsBroker` object.

`nats` is a configuration for NATS broker. It does not support history/recovery/cache.

#### `broker.nats.url`

Type: `string`. Default: `nats://localhost:4222`.

`url` is a Nats server URL.

#### `broker.nats.prefix`

Type: `string`. Default: `centrifugo`.

`prefix` allows customizing channel prefix in Nats to work with a single Nats from different
unrelated Centrifugo setups.

#### `broker.nats.dial_timeout`

Type: `Duration`. Default: `1s`.

`dial_timeout` is a timeout for establishing connection to Nats.

#### `broker.nats.write_timeout`

Type: `Duration`. Default: `1s`.

`write_timeout` is a timeout for write operation to Nats.

#### `broker.nats.tls`

Type: `TLSConfig` object.

`tls` for the Nats connection. TLS is not used if nil.

##### `broker.nats.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `broker.nats.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `broker.nats.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `broker.nats.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `broker.nats.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `broker.nats.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `broker.nats.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

#### `broker.nats.allow_wildcards`

Type: `bool`.

`allow_wildcards` allows to enable wildcard subscriptions. By default, wildcard subscriptions
are not allowed. Using wildcard subscriptions can't be combined with join/leave events and presence
because subscriptions do not belong to a concrete channel after with wildcards, while join/leave events
require concrete channel to be published. And presence does not make a lot of sense for wildcard
subscriptions - there could be subscribers which use different mask, but still receive subset of updates.
It's required to use channels without wildcards to for mentioned features to work properly. When
using wildcard subscriptions a special care is needed regarding security - pay additional
attention to a proper permission management.

#### `broker.nats.raw_mode`

Type: `RawModeConfig` object.

`raw_mode` allows enabling raw communication with Nats. When on, Centrifugo subscribes to channels
without adding any prefixes to channel name. Proper prefixes must be managed by the application in this
case. Data consumed from Nats is sent directly to subscribers without any processing. When publishing
to Nats Centrifugo does not add any prefixes to channel names also. Centrifugo features like Publication
tags, Publication ClientInfo, join/leave events are not supported in raw mode.

##### `broker.nats.raw_mode.enabled`

Type: `bool`.

`enabled` enables raw mode when true.

##### `broker.nats.raw_mode.channel_replacements`

Type: `MapStringString`. Default: `{}`.

`channel_replacements` is a map where keys are strings to replace and values are replacements.
For example, you have Centrifugo namespace "chat" and using channel "chat:index", but you want to
use channel "chat.index" in Nats. Then you can define SymbolReplacements map like this: {":": "."}.
In this case Centrifugo will replace all ":" symbols in channel name with "." before sending to Nats.
Broker keeps reverse mapping to the original channel to broadcast to proper channels when processing
messages received from Nats.

##### `broker.nats.raw_mode.prefix`

Type: `string`.

`prefix` is a string that will be added to all channels when publishing messages to Nats, subscribing
to channels in Nats. It's also stripped from channel name when processing messages received from Nats.
By default, no prefix is used.

## `presence_manager`

Type: `PresenceManager` object.

`presence_manager` allows to configure a presence manager to use. Presence manager is responsible for
presence information storage and retrieval. By default, memory PresenceManager is used. Memory
PresenceManager is superfast, but it's not distributed. Redis PresenceManager provides a seamless
horizontal scalability.

### `presence_manager.enabled`

Type: `bool`.

No documentation available.

### `presence_manager.type`

Type: `string`. Default: `memory`.

`type` of presence manager to use. Can be "memory" or "redis" at this point.

### `presence_manager.redis`

Type: `RedisPresenceManager` object.

`redis` is a configuration for "redis" broker.

#### `presence_manager.redis.address`

Type: `[]string`. Default: `redis://127.0.0.1:6379`.

No documentation available.

#### `presence_manager.redis.prefix`

Type: `string`. Default: `centrifugo`.

No documentation available.

#### `presence_manager.redis.connect_timeout`

Type: `Duration`. Default: `1s`.

No documentation available.

#### `presence_manager.redis.io_timeout`

Type: `Duration`. Default: `4s`.

No documentation available.

#### `presence_manager.redis.db`

Type: `int`. Default: `0`.

No documentation available.

#### `presence_manager.redis.user`

Type: `string`.

No documentation available.

#### `presence_manager.redis.password`

Type: `string`.

No documentation available.

#### `presence_manager.redis.client_name`

Type: `string`.

No documentation available.

#### `presence_manager.redis.force_resp2`

Type: `bool`.

No documentation available.

#### `presence_manager.redis.cluster_address`

Type: `[]string`.

No documentation available.

#### `presence_manager.redis.tls`

Type: `TLSConfig` object.

No documentation available.

##### `presence_manager.redis.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `presence_manager.redis.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `presence_manager.redis.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `presence_manager.redis.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `presence_manager.redis.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `presence_manager.redis.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `presence_manager.redis.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

#### `presence_manager.redis.sentinel_address`

Type: `[]string`.

No documentation available.

#### `presence_manager.redis.sentinel_user`

Type: `string`.

No documentation available.

#### `presence_manager.redis.sentinel_password`

Type: `string`.

No documentation available.

#### `presence_manager.redis.sentinel_master_name`

Type: `string`.

No documentation available.

#### `presence_manager.redis.sentinel_client_name`

Type: `string`.

No documentation available.

#### `presence_manager.redis.sentinel_tls`

Type: `TLSConfig` object.

No documentation available.

##### `presence_manager.redis.sentinel_tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `presence_manager.redis.sentinel_tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `presence_manager.redis.sentinel_tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `presence_manager.redis.sentinel_tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `presence_manager.redis.sentinel_tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `presence_manager.redis.sentinel_tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `presence_manager.redis.sentinel_tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

#### `presence_manager.redis.presence_ttl`

Type: `Duration`. Default: `60s`.

No documentation available.

#### `presence_manager.redis.presence_hash_field_ttl`

Type: `bool`.

No documentation available.

#### `presence_manager.redis.presence_user_mapping`

Type: `bool`.

No documentation available.

## `client`

Type: `Client` object.

`client` contains real-time client connection related configuration.

### `client.proxy`

Type: `ClientProxyContainer` object.

`proxy` is a configuration for client connection-wide proxies.

#### `client.proxy.connect`

Type: `ConnectProxy` object.

`connect` proxy when enabled is used to proxy connect requests from client to the application backend.
Only requests without JWT token are proxied at this point.

##### `client.proxy.connect.enabled`

Type: `bool`.

`enabled` must be true to tell Centrifugo to enable the configured proxy.

##### `client.proxy.connect.endpoint`

Type: `string`.

No documentation available.

##### `client.proxy.connect.timeout`

Type: `Duration`. Default: `1s`.

No documentation available.

##### `client.proxy.connect.http_headers`

Type: `[]string`.

No documentation available.

##### `client.proxy.connect.grpc_metadata`

Type: `[]string`.

No documentation available.

##### `client.proxy.connect.binary_encoding`

Type: `bool`.

No documentation available.

##### `client.proxy.connect.include_connection_meta`

Type: `bool`.

No documentation available.

##### `client.proxy.connect.http`

Type: `ProxyCommonHTTP` object.

No documentation available.

##### `client.proxy.connect.http.tls`

Type: `TLSConfig` object.

No documentation available.

##### `client.proxy.connect.http.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `client.proxy.connect.http.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `client.proxy.connect.http.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `client.proxy.connect.http.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `client.proxy.connect.http.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `client.proxy.connect.http.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `client.proxy.connect.http.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `client.proxy.connect.http.static_headers`

Type: `MapStringString`. Default: `{}`.

`static_headers` is a static set of key/value pairs to attach to HTTP proxy request as
headers. Headers received from HTTP client request or metadata from GRPC client request
both have priority over values set in StaticHttpHeaders map.

##### `client.proxy.connect.http.status_to_code_transforms`

Type: `[]HttpStatusToCodeTransform` object. Default: `[]`.

Status transforms allow to map HTTP status codes from proxy to Disconnect or Error messages.

##### `client.proxy.connect.http.status_to_code_transforms[].status_code`

Type: `int`.

No documentation available.

##### `client.proxy.connect.http.status_to_code_transforms[].to_error`

Type: `TransformError` object.

No documentation available.

##### `client.proxy.connect.http.status_to_code_transforms[].to_error.code`

Type: `uint32`.

No documentation available.

##### `client.proxy.connect.http.status_to_code_transforms[].to_error.message`

Type: `string`.

No documentation available.

##### `client.proxy.connect.http.status_to_code_transforms[].to_error.temporary`

Type: `bool`.

No documentation available.

##### `client.proxy.connect.http.status_to_code_transforms[].to_disconnect`

Type: `TransformDisconnect` object.

No documentation available.

##### `client.proxy.connect.http.status_to_code_transforms[].to_disconnect.code`

Type: `uint32`.

No documentation available.

##### `client.proxy.connect.http.status_to_code_transforms[].to_disconnect.reason`

Type: `string`.

No documentation available.

##### `client.proxy.connect.grpc`

Type: `ProxyCommonGRPC` object.

No documentation available.

##### `client.proxy.connect.grpc.tls`

Type: `TLSConfig` object.

`tls` is a common configuration for GRPC TLS.

##### `client.proxy.connect.grpc.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `client.proxy.connect.grpc.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `client.proxy.connect.grpc.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `client.proxy.connect.grpc.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `client.proxy.connect.grpc.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `client.proxy.connect.grpc.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `client.proxy.connect.grpc.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `client.proxy.connect.grpc.credentials_key`

Type: `string`.

`credentials_key` is a custom key to add into per-RPC credentials.

##### `client.proxy.connect.grpc.credentials_value`

Type: `string`.

GrpcCredentialsValue is a custom value for GrpcCredentialsKey.

##### `client.proxy.connect.grpc.compression`

Type: `bool`.

`compression` enables compression for outgoing calls (gzip).

#### `client.proxy.refresh`

Type: `RefreshProxy` object.

`refresh` proxy when enabled is used to proxy client connection refresh decisions to the application backend.

##### `client.proxy.refresh.enabled`

Type: `bool`.

`enabled` must be true to tell Centrifugo to enable the configured proxy.

##### `client.proxy.refresh.endpoint`

Type: `string`.

No documentation available.

##### `client.proxy.refresh.timeout`

Type: `Duration`. Default: `1s`.

No documentation available.

##### `client.proxy.refresh.http_headers`

Type: `[]string`.

No documentation available.

##### `client.proxy.refresh.grpc_metadata`

Type: `[]string`.

No documentation available.

##### `client.proxy.refresh.binary_encoding`

Type: `bool`.

No documentation available.

##### `client.proxy.refresh.include_connection_meta`

Type: `bool`.

No documentation available.

##### `client.proxy.refresh.http`

Type: `ProxyCommonHTTP` object.

No documentation available.

##### `client.proxy.refresh.http.tls`

Type: `TLSConfig` object.

No documentation available.

##### `client.proxy.refresh.http.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `client.proxy.refresh.http.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `client.proxy.refresh.http.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `client.proxy.refresh.http.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `client.proxy.refresh.http.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `client.proxy.refresh.http.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `client.proxy.refresh.http.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `client.proxy.refresh.http.static_headers`

Type: `MapStringString`. Default: `{}`.

`static_headers` is a static set of key/value pairs to attach to HTTP proxy request as
headers. Headers received from HTTP client request or metadata from GRPC client request
both have priority over values set in StaticHttpHeaders map.

##### `client.proxy.refresh.http.status_to_code_transforms`

Type: `[]HttpStatusToCodeTransform` object. Default: `[]`.

Status transforms allow to map HTTP status codes from proxy to Disconnect or Error messages.

##### `client.proxy.refresh.http.status_to_code_transforms[].status_code`

Type: `int`.

No documentation available.

##### `client.proxy.refresh.http.status_to_code_transforms[].to_error`

Type: `TransformError` object.

No documentation available.

##### `client.proxy.refresh.http.status_to_code_transforms[].to_error.code`

Type: `uint32`.

No documentation available.

##### `client.proxy.refresh.http.status_to_code_transforms[].to_error.message`

Type: `string`.

No documentation available.

##### `client.proxy.refresh.http.status_to_code_transforms[].to_error.temporary`

Type: `bool`.

No documentation available.

##### `client.proxy.refresh.http.status_to_code_transforms[].to_disconnect`

Type: `TransformDisconnect` object.

No documentation available.

##### `client.proxy.refresh.http.status_to_code_transforms[].to_disconnect.code`

Type: `uint32`.

No documentation available.

##### `client.proxy.refresh.http.status_to_code_transforms[].to_disconnect.reason`

Type: `string`.

No documentation available.

##### `client.proxy.refresh.grpc`

Type: `ProxyCommonGRPC` object.

No documentation available.

##### `client.proxy.refresh.grpc.tls`

Type: `TLSConfig` object.

`tls` is a common configuration for GRPC TLS.

##### `client.proxy.refresh.grpc.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `client.proxy.refresh.grpc.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `client.proxy.refresh.grpc.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `client.proxy.refresh.grpc.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `client.proxy.refresh.grpc.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `client.proxy.refresh.grpc.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `client.proxy.refresh.grpc.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `client.proxy.refresh.grpc.credentials_key`

Type: `string`.

`credentials_key` is a custom key to add into per-RPC credentials.

##### `client.proxy.refresh.grpc.credentials_value`

Type: `string`.

GrpcCredentialsValue is a custom value for GrpcCredentialsKey.

##### `client.proxy.refresh.grpc.compression`

Type: `bool`.

`compression` enables compression for outgoing calls (gzip).

### `client.allowed_origins`

Type: `[]string`.

`allowed_origins` is a list of allowed origins for client connections.

### `client.token`

Type: `Token` object.

`token` is a configuration for token generation and verification. When enabled, this configuration
is used for both connection and subscription tokens. See also SubscriptionToken to use a separate
configuration for subscription tokens.

#### `client.token.hmac_secret_key`

Type: `string`.

No documentation available.

#### `client.token.rsa_public_key`

Type: `string`.

No documentation available.

#### `client.token.ecdsa_public_key`

Type: `string`.

No documentation available.

#### `client.token.jwks_public_endpoint`

Type: `string`.

No documentation available.

#### `client.token.audience`

Type: `string`.

No documentation available.

#### `client.token.audience_regex`

Type: `string`.

No documentation available.

#### `client.token.issuer`

Type: `string`.

No documentation available.

#### `client.token.issuer_regex`

Type: `string`.

No documentation available.

#### `client.token.user_id_claim`

Type: `string`.

No documentation available.

### `client.subscription_token`

Type: `SubscriptionToken` object.

`subscription_token` is a configuration for subscription token generation and verification. When enabled,
Centrifugo will use this configuration for subscription tokens only. Configuration in Token is then only
used for connection tokens.

#### `client.subscription_token.enabled`

Type: `bool`.

No documentation available.

#### `client.subscription_token.hmac_secret_key`

Type: `string`.

No documentation available.

#### `client.subscription_token.rsa_public_key`

Type: `string`.

No documentation available.

#### `client.subscription_token.ecdsa_public_key`

Type: `string`.

No documentation available.

#### `client.subscription_token.jwks_public_endpoint`

Type: `string`.

No documentation available.

#### `client.subscription_token.audience`

Type: `string`.

No documentation available.

#### `client.subscription_token.audience_regex`

Type: `string`.

No documentation available.

#### `client.subscription_token.issuer`

Type: `string`.

No documentation available.

#### `client.subscription_token.issuer_regex`

Type: `string`.

No documentation available.

#### `client.subscription_token.user_id_claim`

Type: `string`.

No documentation available.

### `client.allow_anonymous_connect_without_token`

Type: `bool`.

`allow_anonymous_connect_without_token` allows to connect to Centrifugo without a token. In this case connection will
be accepted but client will be anonymous (i.e. will have empty user ID).

### `client.disallow_anonymous_connection_tokens`

Type: `bool`.

`disallow_anonymous_connection_tokens` disallows anonymous connection tokens. When enabled, Centrifugo will not
accept connection tokens with empty user ID.

### `client.ping_interval`

Type: `Duration`. Default: `25s`.

No documentation available.

### `client.pong_timeout`

Type: `Duration`. Default: `8s`.

No documentation available.

### `client.expired_close_delay`

Type: `Duration`. Default: `25s`.

No documentation available.

### `client.expired_sub_close_delay`

Type: `Duration`. Default: `25s`.

No documentation available.

### `client.stale_close_delay`

Type: `Duration`. Default: `10s`.

No documentation available.

### `client.channel_limit`

Type: `int`. Default: `128`.

No documentation available.

### `client.queue_max_size`

Type: `int`. Default: `1048576`.

No documentation available.

### `client.presence_update_interval`

Type: `Duration`. Default: `27s`.

No documentation available.

### `client.concurrency`

Type: `int`.

No documentation available.

### `client.channel_position_check_delay`

Type: `Duration`. Default: `40s`.

No documentation available.

### `client.channel_position_max_time_lag`

Type: `Duration`.

No documentation available.

### `client.connection_limit`

Type: `int`.

No documentation available.

### `client.user_connection_limit`

Type: `int`.

No documentation available.

### `client.connection_rate_limit`

Type: `int`.

No documentation available.

### `client.connect_include_server_time`

Type: `bool`.

No documentation available.

### `client.history_max_publication_limit`

Type: `int`. Default: `300`.

No documentation available.

### `client.recovery_max_publication_limit`

Type: `int`. Default: `300`.

No documentation available.

### `client.insecure_skip_token_signature_verify`

Type: `bool`.

No documentation available.

### `client.user_id_http_header`

Type: `string`.

No documentation available.

### `client.insecure`

Type: `bool`.

No documentation available.

### `client.subscribe_to_user_personal_channel`

Type: `SubscribeToUserPersonalChannel` object.

`subscribe_to_user_personal_channel` is a configuration for a feature to automatically subscribe user to a personal channel
using server-side subscription.

#### `client.subscribe_to_user_personal_channel.enabled`

Type: `bool`.

No documentation available.

#### `client.subscribe_to_user_personal_channel.personal_channel_namespace`

Type: `string`.

No documentation available.

#### `client.subscribe_to_user_personal_channel.single_connection`

Type: `bool`.

No documentation available.

### `client.connect_code_to_unidirectional_disconnect`

Type: `ConnectCodeToUnidirectionalDisconnect` object.

ConnectCodeToDisconnect is a configuration for a feature to transform connect error codes to the disconnect code
for unidirectional transports.

#### `client.connect_code_to_unidirectional_disconnect.enabled`

Type: `bool`.

No documentation available.

#### `client.connect_code_to_unidirectional_disconnect.transforms`

Type: `[]UniConnectCodeToDisconnectTransform` object. Default: `[]`.

No documentation available.

##### `client.connect_code_to_unidirectional_disconnect.transforms[].code`

Type: `uint32`.

No documentation available.

##### `client.connect_code_to_unidirectional_disconnect.transforms[].to`

Type: `TransformDisconnect` object.

No documentation available.

##### `client.connect_code_to_unidirectional_disconnect.transforms[].to.code`

Type: `uint32`.

No documentation available.

##### `client.connect_code_to_unidirectional_disconnect.transforms[].to.reason`

Type: `string`.

No documentation available.

## `channel`

Type: `Channel` object.

`channel` contains real-time channel related configuration.

### `channel.proxy`

Type: `ChannelProxyContainer` object.

`proxy` configuration for channel-related events. All types inside can be referenced by the name "default".

#### `channel.proxy.subscribe`

Type: `Proxy` object.

No documentation available.

##### `channel.proxy.subscribe.endpoint`

Type: `string`.

`endpoint` - HTTP address or GRPC service endpoint.

##### `channel.proxy.subscribe.timeout`

Type: `Duration`. Default: `1s`.

`timeout` for proxy request.

##### `channel.proxy.subscribe.http_headers`

Type: `[]string`.

No documentation available.

##### `channel.proxy.subscribe.grpc_metadata`

Type: `[]string`.

No documentation available.

##### `channel.proxy.subscribe.binary_encoding`

Type: `bool`.

No documentation available.

##### `channel.proxy.subscribe.include_connection_meta`

Type: `bool`.

No documentation available.

##### `channel.proxy.subscribe.http`

Type: `ProxyCommonHTTP` object.

No documentation available.

##### `channel.proxy.subscribe.http.tls`

Type: `TLSConfig` object.

No documentation available.

##### `channel.proxy.subscribe.http.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `channel.proxy.subscribe.http.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `channel.proxy.subscribe.http.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `channel.proxy.subscribe.http.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `channel.proxy.subscribe.http.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `channel.proxy.subscribe.http.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `channel.proxy.subscribe.http.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `channel.proxy.subscribe.http.static_headers`

Type: `MapStringString`. Default: `{}`.

`static_headers` is a static set of key/value pairs to attach to HTTP proxy request as
headers. Headers received from HTTP client request or metadata from GRPC client request
both have priority over values set in StaticHttpHeaders map.

##### `channel.proxy.subscribe.http.status_to_code_transforms`

Type: `[]HttpStatusToCodeTransform` object. Default: `[]`.

Status transforms allow to map HTTP status codes from proxy to Disconnect or Error messages.

##### `channel.proxy.subscribe.http.status_to_code_transforms[].status_code`

Type: `int`.

No documentation available.

##### `channel.proxy.subscribe.http.status_to_code_transforms[].to_error`

Type: `TransformError` object.

No documentation available.

##### `channel.proxy.subscribe.http.status_to_code_transforms[].to_error.code`

Type: `uint32`.

No documentation available.

##### `channel.proxy.subscribe.http.status_to_code_transforms[].to_error.message`

Type: `string`.

No documentation available.

##### `channel.proxy.subscribe.http.status_to_code_transforms[].to_error.temporary`

Type: `bool`.

No documentation available.

##### `channel.proxy.subscribe.http.status_to_code_transforms[].to_disconnect`

Type: `TransformDisconnect` object.

No documentation available.

##### `channel.proxy.subscribe.http.status_to_code_transforms[].to_disconnect.code`

Type: `uint32`.

No documentation available.

##### `channel.proxy.subscribe.http.status_to_code_transforms[].to_disconnect.reason`

Type: `string`.

No documentation available.

##### `channel.proxy.subscribe.grpc`

Type: `ProxyCommonGRPC` object.

No documentation available.

##### `channel.proxy.subscribe.grpc.tls`

Type: `TLSConfig` object.

`tls` is a common configuration for GRPC TLS.

##### `channel.proxy.subscribe.grpc.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `channel.proxy.subscribe.grpc.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `channel.proxy.subscribe.grpc.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `channel.proxy.subscribe.grpc.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `channel.proxy.subscribe.grpc.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `channel.proxy.subscribe.grpc.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `channel.proxy.subscribe.grpc.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `channel.proxy.subscribe.grpc.credentials_key`

Type: `string`.

`credentials_key` is a custom key to add into per-RPC credentials.

##### `channel.proxy.subscribe.grpc.credentials_value`

Type: `string`.

GrpcCredentialsValue is a custom value for GrpcCredentialsKey.

##### `channel.proxy.subscribe.grpc.compression`

Type: `bool`.

`compression` enables compression for outgoing calls (gzip).

#### `channel.proxy.publish`

Type: `Proxy` object.

No documentation available.

##### `channel.proxy.publish.endpoint`

Type: `string`.

`endpoint` - HTTP address or GRPC service endpoint.

##### `channel.proxy.publish.timeout`

Type: `Duration`. Default: `1s`.

`timeout` for proxy request.

##### `channel.proxy.publish.http_headers`

Type: `[]string`.

No documentation available.

##### `channel.proxy.publish.grpc_metadata`

Type: `[]string`.

No documentation available.

##### `channel.proxy.publish.binary_encoding`

Type: `bool`.

No documentation available.

##### `channel.proxy.publish.include_connection_meta`

Type: `bool`.

No documentation available.

##### `channel.proxy.publish.http`

Type: `ProxyCommonHTTP` object.

No documentation available.

##### `channel.proxy.publish.http.tls`

Type: `TLSConfig` object.

No documentation available.

##### `channel.proxy.publish.http.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `channel.proxy.publish.http.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `channel.proxy.publish.http.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `channel.proxy.publish.http.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `channel.proxy.publish.http.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `channel.proxy.publish.http.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `channel.proxy.publish.http.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `channel.proxy.publish.http.static_headers`

Type: `MapStringString`. Default: `{}`.

`static_headers` is a static set of key/value pairs to attach to HTTP proxy request as
headers. Headers received from HTTP client request or metadata from GRPC client request
both have priority over values set in StaticHttpHeaders map.

##### `channel.proxy.publish.http.status_to_code_transforms`

Type: `[]HttpStatusToCodeTransform` object. Default: `[]`.

Status transforms allow to map HTTP status codes from proxy to Disconnect or Error messages.

##### `channel.proxy.publish.http.status_to_code_transforms[].status_code`

Type: `int`.

No documentation available.

##### `channel.proxy.publish.http.status_to_code_transforms[].to_error`

Type: `TransformError` object.

No documentation available.

##### `channel.proxy.publish.http.status_to_code_transforms[].to_error.code`

Type: `uint32`.

No documentation available.

##### `channel.proxy.publish.http.status_to_code_transforms[].to_error.message`

Type: `string`.

No documentation available.

##### `channel.proxy.publish.http.status_to_code_transforms[].to_error.temporary`

Type: `bool`.

No documentation available.

##### `channel.proxy.publish.http.status_to_code_transforms[].to_disconnect`

Type: `TransformDisconnect` object.

No documentation available.

##### `channel.proxy.publish.http.status_to_code_transforms[].to_disconnect.code`

Type: `uint32`.

No documentation available.

##### `channel.proxy.publish.http.status_to_code_transforms[].to_disconnect.reason`

Type: `string`.

No documentation available.

##### `channel.proxy.publish.grpc`

Type: `ProxyCommonGRPC` object.

No documentation available.

##### `channel.proxy.publish.grpc.tls`

Type: `TLSConfig` object.

`tls` is a common configuration for GRPC TLS.

##### `channel.proxy.publish.grpc.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `channel.proxy.publish.grpc.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `channel.proxy.publish.grpc.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `channel.proxy.publish.grpc.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `channel.proxy.publish.grpc.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `channel.proxy.publish.grpc.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `channel.proxy.publish.grpc.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `channel.proxy.publish.grpc.credentials_key`

Type: `string`.

`credentials_key` is a custom key to add into per-RPC credentials.

##### `channel.proxy.publish.grpc.credentials_value`

Type: `string`.

GrpcCredentialsValue is a custom value for GrpcCredentialsKey.

##### `channel.proxy.publish.grpc.compression`

Type: `bool`.

`compression` enables compression for outgoing calls (gzip).

#### `channel.proxy.sub_refresh`

Type: `Proxy` object.

No documentation available.

##### `channel.proxy.sub_refresh.endpoint`

Type: `string`.

`endpoint` - HTTP address or GRPC service endpoint.

##### `channel.proxy.sub_refresh.timeout`

Type: `Duration`. Default: `1s`.

`timeout` for proxy request.

##### `channel.proxy.sub_refresh.http_headers`

Type: `[]string`.

No documentation available.

##### `channel.proxy.sub_refresh.grpc_metadata`

Type: `[]string`.

No documentation available.

##### `channel.proxy.sub_refresh.binary_encoding`

Type: `bool`.

No documentation available.

##### `channel.proxy.sub_refresh.include_connection_meta`

Type: `bool`.

No documentation available.

##### `channel.proxy.sub_refresh.http`

Type: `ProxyCommonHTTP` object.

No documentation available.

##### `channel.proxy.sub_refresh.http.tls`

Type: `TLSConfig` object.

No documentation available.

##### `channel.proxy.sub_refresh.http.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `channel.proxy.sub_refresh.http.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `channel.proxy.sub_refresh.http.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `channel.proxy.sub_refresh.http.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `channel.proxy.sub_refresh.http.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `channel.proxy.sub_refresh.http.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `channel.proxy.sub_refresh.http.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `channel.proxy.sub_refresh.http.static_headers`

Type: `MapStringString`. Default: `{}`.

`static_headers` is a static set of key/value pairs to attach to HTTP proxy request as
headers. Headers received from HTTP client request or metadata from GRPC client request
both have priority over values set in StaticHttpHeaders map.

##### `channel.proxy.sub_refresh.http.status_to_code_transforms`

Type: `[]HttpStatusToCodeTransform` object. Default: `[]`.

Status transforms allow to map HTTP status codes from proxy to Disconnect or Error messages.

##### `channel.proxy.sub_refresh.http.status_to_code_transforms[].status_code`

Type: `int`.

No documentation available.

##### `channel.proxy.sub_refresh.http.status_to_code_transforms[].to_error`

Type: `TransformError` object.

No documentation available.

##### `channel.proxy.sub_refresh.http.status_to_code_transforms[].to_error.code`

Type: `uint32`.

No documentation available.

##### `channel.proxy.sub_refresh.http.status_to_code_transforms[].to_error.message`

Type: `string`.

No documentation available.

##### `channel.proxy.sub_refresh.http.status_to_code_transforms[].to_error.temporary`

Type: `bool`.

No documentation available.

##### `channel.proxy.sub_refresh.http.status_to_code_transforms[].to_disconnect`

Type: `TransformDisconnect` object.

No documentation available.

##### `channel.proxy.sub_refresh.http.status_to_code_transforms[].to_disconnect.code`

Type: `uint32`.

No documentation available.

##### `channel.proxy.sub_refresh.http.status_to_code_transforms[].to_disconnect.reason`

Type: `string`.

No documentation available.

##### `channel.proxy.sub_refresh.grpc`

Type: `ProxyCommonGRPC` object.

No documentation available.

##### `channel.proxy.sub_refresh.grpc.tls`

Type: `TLSConfig` object.

`tls` is a common configuration for GRPC TLS.

##### `channel.proxy.sub_refresh.grpc.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `channel.proxy.sub_refresh.grpc.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `channel.proxy.sub_refresh.grpc.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `channel.proxy.sub_refresh.grpc.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `channel.proxy.sub_refresh.grpc.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `channel.proxy.sub_refresh.grpc.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `channel.proxy.sub_refresh.grpc.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `channel.proxy.sub_refresh.grpc.credentials_key`

Type: `string`.

`credentials_key` is a custom key to add into per-RPC credentials.

##### `channel.proxy.sub_refresh.grpc.credentials_value`

Type: `string`.

GrpcCredentialsValue is a custom value for GrpcCredentialsKey.

##### `channel.proxy.sub_refresh.grpc.compression`

Type: `bool`.

`compression` enables compression for outgoing calls (gzip).

#### `channel.proxy.subscribe_stream`

Type: `Proxy` object.

No documentation available.

##### `channel.proxy.subscribe_stream.endpoint`

Type: `string`.

`endpoint` - HTTP address or GRPC service endpoint.

##### `channel.proxy.subscribe_stream.timeout`

Type: `Duration`. Default: `1s`.

`timeout` for proxy request.

##### `channel.proxy.subscribe_stream.http_headers`

Type: `[]string`.

No documentation available.

##### `channel.proxy.subscribe_stream.grpc_metadata`

Type: `[]string`.

No documentation available.

##### `channel.proxy.subscribe_stream.binary_encoding`

Type: `bool`.

No documentation available.

##### `channel.proxy.subscribe_stream.include_connection_meta`

Type: `bool`.

No documentation available.

##### `channel.proxy.subscribe_stream.http`

Type: `ProxyCommonHTTP` object.

No documentation available.

##### `channel.proxy.subscribe_stream.http.tls`

Type: `TLSConfig` object.

No documentation available.

##### `channel.proxy.subscribe_stream.http.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `channel.proxy.subscribe_stream.http.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `channel.proxy.subscribe_stream.http.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `channel.proxy.subscribe_stream.http.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `channel.proxy.subscribe_stream.http.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `channel.proxy.subscribe_stream.http.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `channel.proxy.subscribe_stream.http.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `channel.proxy.subscribe_stream.http.static_headers`

Type: `MapStringString`. Default: `{}`.

`static_headers` is a static set of key/value pairs to attach to HTTP proxy request as
headers. Headers received from HTTP client request or metadata from GRPC client request
both have priority over values set in StaticHttpHeaders map.

##### `channel.proxy.subscribe_stream.http.status_to_code_transforms`

Type: `[]HttpStatusToCodeTransform` object. Default: `[]`.

Status transforms allow to map HTTP status codes from proxy to Disconnect or Error messages.

##### `channel.proxy.subscribe_stream.http.status_to_code_transforms[].status_code`

Type: `int`.

No documentation available.

##### `channel.proxy.subscribe_stream.http.status_to_code_transforms[].to_error`

Type: `TransformError` object.

No documentation available.

##### `channel.proxy.subscribe_stream.http.status_to_code_transforms[].to_error.code`

Type: `uint32`.

No documentation available.

##### `channel.proxy.subscribe_stream.http.status_to_code_transforms[].to_error.message`

Type: `string`.

No documentation available.

##### `channel.proxy.subscribe_stream.http.status_to_code_transforms[].to_error.temporary`

Type: `bool`.

No documentation available.

##### `channel.proxy.subscribe_stream.http.status_to_code_transforms[].to_disconnect`

Type: `TransformDisconnect` object.

No documentation available.

##### `channel.proxy.subscribe_stream.http.status_to_code_transforms[].to_disconnect.code`

Type: `uint32`.

No documentation available.

##### `channel.proxy.subscribe_stream.http.status_to_code_transforms[].to_disconnect.reason`

Type: `string`.

No documentation available.

##### `channel.proxy.subscribe_stream.grpc`

Type: `ProxyCommonGRPC` object.

No documentation available.

##### `channel.proxy.subscribe_stream.grpc.tls`

Type: `TLSConfig` object.

`tls` is a common configuration for GRPC TLS.

##### `channel.proxy.subscribe_stream.grpc.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `channel.proxy.subscribe_stream.grpc.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `channel.proxy.subscribe_stream.grpc.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `channel.proxy.subscribe_stream.grpc.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `channel.proxy.subscribe_stream.grpc.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `channel.proxy.subscribe_stream.grpc.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `channel.proxy.subscribe_stream.grpc.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `channel.proxy.subscribe_stream.grpc.credentials_key`

Type: `string`.

`credentials_key` is a custom key to add into per-RPC credentials.

##### `channel.proxy.subscribe_stream.grpc.credentials_value`

Type: `string`.

GrpcCredentialsValue is a custom value for GrpcCredentialsKey.

##### `channel.proxy.subscribe_stream.grpc.compression`

Type: `bool`.

`compression` enables compression for outgoing calls (gzip).

### `channel.without_namespace`

Type: `ChannelOptions` object.

`without_namespace` is a configuration of channels options for channels which do not have namespace.
Generally, we recommend always use channel namespaces but this option can be useful for simple setups.

#### `channel.without_namespace.presence`

Type: `bool`.

`presence` turns on presence information for channel. Presence has
information about all clients currently subscribed to a channel.

#### `channel.without_namespace.join_leave`

Type: `bool`.

`join_leave` turns on join/leave messages for a channel.
When client subscribes on a channel join message sent to all
subscribers in this channel (including current client). When client
leaves channel (unsubscribes) leave message sent. This option does
not fit well for channels with many subscribers because every
subscribe/unsubscribe event results into join/leave event broadcast
to all other active subscribers thus overloads server with tons of
messages. Use accurately for channels with small number of active
subscribers.

#### `channel.without_namespace.force_push_join_leave`

Type: `bool`.

`force_push_join_leave` forces sending join/leave messages towards subscribers.

#### `channel.without_namespace.history_size`

Type: `int`.

`history_size` determines max amount of history messages for a channel,
Zero value means no history for channel. Centrifuge history has an
auxiliary role with current Engines – it can not replace your backend
persistent storage.

#### `channel.without_namespace.history_ttl`

Type: `Duration`.

`history_ttl` is a time to live for history cache. Server maintains a window of
messages in memory (or in Redis with Redis engine), to prevent infinite memory
grows it's important to remove history for inactive channels.

#### `channel.without_namespace.history_meta_ttl`

Type: `Duration`.

`history_meta_ttl` is a time to live for history stream meta information. Must be
much larger than HistoryTTL in common scenario. If zero, then we use global value
set over default_history_meta_ttl on configuration top level.

#### `channel.without_namespace.force_positioning`

Type: `bool`.

`force_positioning` enables client positioning. This means that StreamPosition
will be exposed to the client and server will look that no messages from
PUB/SUB layer lost. In the loss found – client is disconnected (or unsubscribed)
with reconnect (resubscribe) code.

#### `channel.without_namespace.allow_positioning`

Type: `bool`.

`allow_positioning` allows positioning when client asks about it.

#### `channel.without_namespace.force_recovery`

Type: `bool`.

`force_recovery` enables recovery mechanism for channels. This means that
server will try to recover missed messages for resubscribing client.
This option uses publications from history and must be used with reasonable
HistorySize and HistoryTTL configuration.

#### `channel.without_namespace.allow_recovery`

Type: `bool`.

`allow_recovery` allows recovery when client asks about it.

#### `channel.without_namespace.force_recovery_mode`

Type: `string`.

`force_recovery_mode` can set the recovery mode for all channel subscribers in the namespace which use recovery.

#### `channel.without_namespace.allowed_delta_types`

Type: `[]centrifuge.DeltaType`.

`allowed_delta_types` is non-empty contains slice of allowed delta types for subscribers to use.

#### `channel.without_namespace.delta_publish`

Type: `bool`.

`delta_publish` enables delta publish mechanism for all messages published in namespace channels
without explicit flag usage in publish API request. Setting this option does not guarantee that
publication will be compressed when going towards subscribers – it still depends on subscriber
connection options and whether Centrifugo Node is able to find previous publication in channel.

#### `channel.without_namespace.allow_subscribe_for_anonymous`

Type: `bool`.

`allow_subscribe_for_anonymous` ...

#### `channel.without_namespace.allow_subscribe_for_client`

Type: `bool`.

`allow_subscribe_for_client` ...

#### `channel.without_namespace.allow_publish_for_anonymous`

Type: `bool`.

`allow_publish_for_anonymous` ...

#### `channel.without_namespace.allow_publish_for_subscriber`

Type: `bool`.

`allow_publish_for_subscriber` ...

#### `channel.without_namespace.allow_publish_for_client`

Type: `bool`.

`allow_publish_for_client` ...

#### `channel.without_namespace.allow_presence_for_anonymous`

Type: `bool`.

`allow_presence_for_anonymous` ...

#### `channel.without_namespace.allow_presence_for_subscriber`

Type: `bool`.

`allow_presence_for_subscriber` ...

#### `channel.without_namespace.allow_presence_for_client`

Type: `bool`.

`allow_presence_for_client` ...

#### `channel.without_namespace.allow_history_for_anonymous`

Type: `bool`.

`allow_history_for_anonymous` ...

#### `channel.without_namespace.allow_history_for_subscriber`

Type: `bool`.

`allow_history_for_subscriber` ...

#### `channel.without_namespace.allow_history_for_client`

Type: `bool`.

`allow_history_for_client` ...

#### `channel.without_namespace.allow_user_limited_channels`

Type: `bool`.

`allow_user_limited_channels` ...

#### `channel.without_namespace.channel_regex`

Type: `string`.

`channel_regex` ...

#### `channel.without_namespace.subscribe_proxy_enabled`

Type: `bool`.

`subscribe_proxy_enabled` turns on using proxy for subscribe operations in namespace.

#### `channel.without_namespace.subscribe_proxy_name`

Type: `string`. Default: `default`.

`subscribe_proxy_name` of proxy to use for subscribe operations in namespace.

#### `channel.without_namespace.publish_proxy_enabled`

Type: `bool`.

`publish_proxy_enabled` turns on using proxy for publish operations in namespace.

#### `channel.without_namespace.publish_proxy_name`

Type: `string`. Default: `default`.

`publish_proxy_name` of proxy to use for publish operations in namespace.

#### `channel.without_namespace.sub_refresh_proxy_enabled`

Type: `bool`.

`sub_refresh_proxy_enabled` turns on using proxy for sub refresh operations in namespace.

#### `channel.without_namespace.sub_refresh_proxy_name`

Type: `string`. Default: `default`.

`sub_refresh_proxy_name` of proxy to use for sub refresh operations in namespace.

#### `channel.without_namespace.subscribe_stream_proxy_enabled`

Type: `bool`.

`subscribe_stream_proxy_enabled` turns on using proxy for subscribe stream operations in namespace.

#### `channel.without_namespace.subscribe_stream_proxy_name`

Type: `string`. Default: `default`.

`subscribe_stream_proxy_name` of proxy to use for subscribe stream operations in namespace.

#### `channel.without_namespace.subscribe_stream_proxy_bidirectional`

Type: `bool`.

`subscribe_stream_proxy_bidirectional` enables using bidirectional stream proxy for the namespace.

### `channel.namespaces`

Type: `[]ChannelNamespace` object. Default: `[]`.

`namespaces` is a list of channel namespaces. Each channel namespace can have its own set of rules.

#### `channel.namespaces[].name`

Type: `string`.

`name` is a unique namespace name.

#### `channel.namespaces[].presence`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].join_leave`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].force_push_join_leave`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].history_size`

Type: `int`.

No documentation available.

#### `channel.namespaces[].history_ttl`

Type: `Duration`.

No documentation available.

#### `channel.namespaces[].history_meta_ttl`

Type: `Duration`.

No documentation available.

#### `channel.namespaces[].force_positioning`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_positioning`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].force_recovery`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_recovery`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].force_recovery_mode`

Type: `string`.

No documentation available.

#### `channel.namespaces[].allowed_delta_types`

Type: `[]centrifuge.DeltaType`.

No documentation available.

#### `channel.namespaces[].delta_publish`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_subscribe_for_anonymous`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_subscribe_for_client`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_publish_for_anonymous`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_publish_for_subscriber`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_publish_for_client`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_presence_for_anonymous`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_presence_for_subscriber`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_presence_for_client`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_history_for_anonymous`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_history_for_subscriber`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_history_for_client`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].allow_user_limited_channels`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].channel_regex`

Type: `string`.

No documentation available.

#### `channel.namespaces[].subscribe_proxy_enabled`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].subscribe_proxy_name`

Type: `string`. Default: `default`.

No documentation available.

#### `channel.namespaces[].publish_proxy_enabled`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].publish_proxy_name`

Type: `string`. Default: `default`.

No documentation available.

#### `channel.namespaces[].sub_refresh_proxy_enabled`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].sub_refresh_proxy_name`

Type: `string`. Default: `default`.

No documentation available.

#### `channel.namespaces[].subscribe_stream_proxy_enabled`

Type: `bool`.

No documentation available.

#### `channel.namespaces[].subscribe_stream_proxy_name`

Type: `string`. Default: `default`.

No documentation available.

#### `channel.namespaces[].subscribe_stream_proxy_bidirectional`

Type: `bool`.

No documentation available.

### `channel.history_meta_ttl`

Type: `Duration`. Default: `720h`.

HistoryTTL is a time how long to keep history meta information. This is a global option for all channels,
but it can be overridden in channel namespace.

### `channel.max_length`

Type: `int`. Default: `255`.

No documentation available.

### `channel.private_prefix`

Type: `string`. Default: `$`.

No documentation available.

### `channel.namespace_boundary`

Type: `string`. Default: `:`.

No documentation available.

### `channel.user_boundary`

Type: `string`. Default: `#`.

No documentation available.

### `channel.user_separator`

Type: `string`. Default: `,`.

No documentation available.

## `rpc`

Type: `RPC` object.

`rpc` is a configuration for client RPC calls.

### `rpc.proxy`

Type: `Proxy` object.

`proxy` configuration for rpc-related events. Can be referenced by the name "default".

#### `rpc.proxy.endpoint`

Type: `string`.

`endpoint` - HTTP address or GRPC service endpoint.

#### `rpc.proxy.timeout`

Type: `Duration`. Default: `1s`.

`timeout` for proxy request.

#### `rpc.proxy.http_headers`

Type: `[]string`.

No documentation available.

#### `rpc.proxy.grpc_metadata`

Type: `[]string`.

No documentation available.

#### `rpc.proxy.binary_encoding`

Type: `bool`.

No documentation available.

#### `rpc.proxy.include_connection_meta`

Type: `bool`.

No documentation available.

#### `rpc.proxy.http`

Type: `ProxyCommonHTTP` object.

No documentation available.

##### `rpc.proxy.http.tls`

Type: `TLSConfig` object.

No documentation available.

##### `rpc.proxy.http.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `rpc.proxy.http.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `rpc.proxy.http.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `rpc.proxy.http.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `rpc.proxy.http.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `rpc.proxy.http.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `rpc.proxy.http.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `rpc.proxy.http.static_headers`

Type: `MapStringString`. Default: `{}`.

`static_headers` is a static set of key/value pairs to attach to HTTP proxy request as
headers. Headers received from HTTP client request or metadata from GRPC client request
both have priority over values set in StaticHttpHeaders map.

##### `rpc.proxy.http.status_to_code_transforms`

Type: `[]HttpStatusToCodeTransform` object. Default: `[]`.

Status transforms allow to map HTTP status codes from proxy to Disconnect or Error messages.

##### `rpc.proxy.http.status_to_code_transforms[].status_code`

Type: `int`.

No documentation available.

##### `rpc.proxy.http.status_to_code_transforms[].to_error`

Type: `TransformError` object.

No documentation available.

##### `rpc.proxy.http.status_to_code_transforms[].to_error.code`

Type: `uint32`.

No documentation available.

##### `rpc.proxy.http.status_to_code_transforms[].to_error.message`

Type: `string`.

No documentation available.

##### `rpc.proxy.http.status_to_code_transforms[].to_error.temporary`

Type: `bool`.

No documentation available.

##### `rpc.proxy.http.status_to_code_transforms[].to_disconnect`

Type: `TransformDisconnect` object.

No documentation available.

##### `rpc.proxy.http.status_to_code_transforms[].to_disconnect.code`

Type: `uint32`.

No documentation available.

##### `rpc.proxy.http.status_to_code_transforms[].to_disconnect.reason`

Type: `string`.

No documentation available.

#### `rpc.proxy.grpc`

Type: `ProxyCommonGRPC` object.

No documentation available.

##### `rpc.proxy.grpc.tls`

Type: `TLSConfig` object.

`tls` is a common configuration for GRPC TLS.

##### `rpc.proxy.grpc.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `rpc.proxy.grpc.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `rpc.proxy.grpc.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `rpc.proxy.grpc.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `rpc.proxy.grpc.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `rpc.proxy.grpc.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `rpc.proxy.grpc.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

##### `rpc.proxy.grpc.credentials_key`

Type: `string`.

`credentials_key` is a custom key to add into per-RPC credentials.

##### `rpc.proxy.grpc.credentials_value`

Type: `string`.

GrpcCredentialsValue is a custom value for GrpcCredentialsKey.

##### `rpc.proxy.grpc.compression`

Type: `bool`.

`compression` enables compression for outgoing calls (gzip).

### `rpc.without_namespace`

Type: `RpcOptions` object.

`without_namespace` is a configuration of RpcOptions for rpc methods without rpc namespace. Generally,
we recommend always use rpc namespaces but this option can be useful for simple setups.

#### `rpc.without_namespace.proxy_enabled`

Type: `bool`.

`proxy_enabled` allows to enable using RPC proxy for this namespace.

#### `rpc.without_namespace.proxy_name`

Type: `string`. Default: `default`.

`proxy_name` which should be used for RPC namespace.

### `rpc.namespaces`

Type: `[]RpcNamespace` object. Default: `[]`.

RPCNamespaces is a list of rpc namespaces. Each rpc namespace can have its own set of rules.

#### `rpc.namespaces[].name`

Type: `string`.

`name` is a unique rpc namespace name.

#### `rpc.namespaces[].proxy_enabled`

Type: `bool`.

No documentation available.

#### `rpc.namespaces[].proxy_name`

Type: `string`. Default: `default`.

No documentation available.

### `rpc.ping`

Type: `RPCPing` object.

`ping` is a configuration for RPC ping method.

#### `rpc.ping.enabled`

Type: `bool`.

No documentation available.

#### `rpc.ping.method`

Type: `string`. Default: `ping`.

No documentation available.

### `rpc.namespace_boundary`

Type: `string`. Default: `:`.

No documentation available.

## `proxies`

Type: `[]NamedProxy` object. Default: `[]`.

`proxies` is an array of proxies with custom names for the more granular control of channel-related events
in different channel namespaces.

### `proxies[].name`

Type: `string`.

No documentation available.

### `proxies[].endpoint`

Type: `string`.

No documentation available.

### `proxies[].timeout`

Type: `Duration`. Default: `1s`.

No documentation available.

### `proxies[].http_headers`

Type: `[]string`.

No documentation available.

### `proxies[].grpc_metadata`

Type: `[]string`.

No documentation available.

### `proxies[].binary_encoding`

Type: `bool`.

No documentation available.

### `proxies[].include_connection_meta`

Type: `bool`.

No documentation available.

### `proxies[].http`

Type: `ProxyCommonHTTP` object.

No documentation available.

#### `proxies[].http.tls`

Type: `TLSConfig` object.

No documentation available.

##### `proxies[].http.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `proxies[].http.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `proxies[].http.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `proxies[].http.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `proxies[].http.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `proxies[].http.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `proxies[].http.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

#### `proxies[].http.static_headers`

Type: `MapStringString`. Default: `{}`.

`static_headers` is a static set of key/value pairs to attach to HTTP proxy request as
headers. Headers received from HTTP client request or metadata from GRPC client request
both have priority over values set in StaticHttpHeaders map.

#### `proxies[].http.status_to_code_transforms`

Type: `[]HttpStatusToCodeTransform` object. Default: `[]`.

Status transforms allow to map HTTP status codes from proxy to Disconnect or Error messages.

##### `proxies[].http.status_to_code_transforms[].status_code`

Type: `int`.

No documentation available.

##### `proxies[].http.status_to_code_transforms[].to_error`

Type: `TransformError` object.

No documentation available.

##### `proxies[].http.status_to_code_transforms[].to_error.code`

Type: `uint32`.

No documentation available.

##### `proxies[].http.status_to_code_transforms[].to_error.message`

Type: `string`.

No documentation available.

##### `proxies[].http.status_to_code_transforms[].to_error.temporary`

Type: `bool`.

No documentation available.

##### `proxies[].http.status_to_code_transforms[].to_disconnect`

Type: `TransformDisconnect` object.

No documentation available.

##### `proxies[].http.status_to_code_transforms[].to_disconnect.code`

Type: `uint32`.

No documentation available.

##### `proxies[].http.status_to_code_transforms[].to_disconnect.reason`

Type: `string`.

No documentation available.

### `proxies[].grpc`

Type: `ProxyCommonGRPC` object.

No documentation available.

#### `proxies[].grpc.tls`

Type: `TLSConfig` object.

`tls` is a common configuration for GRPC TLS.

##### `proxies[].grpc.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `proxies[].grpc.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `proxies[].grpc.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `proxies[].grpc.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `proxies[].grpc.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `proxies[].grpc.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `proxies[].grpc.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

#### `proxies[].grpc.credentials_key`

Type: `string`.

`credentials_key` is a custom key to add into per-RPC credentials.

#### `proxies[].grpc.credentials_value`

Type: `string`.

GrpcCredentialsValue is a custom value for GrpcCredentialsKey.

#### `proxies[].grpc.compression`

Type: `bool`.

`compression` enables compression for outgoing calls (gzip).

## `http_api`

Type: `HttpAPI` object.

`http_api` is a configuration for HTTP server API. It's enabled by default.

### `http_api.disabled`

Type: `bool`.

No documentation available.

### `http_api.handler_prefix`

Type: `string`. Default: `/api`.

No documentation available.

### `http_api.key`

Type: `string`.

No documentation available.

### `http_api.error_mode`

Type: `string`.

No documentation available.

### `http_api.external`

Type: `bool`.

No documentation available.

### `http_api.insecure`

Type: `bool`.

No documentation available.

## `grpc_api`

Type: `GrpcAPI` object.

`grpc_api` is a configuration for gRPC server API. It's disabled by default.

### `grpc_api.enabled`

Type: `bool`.

No documentation available.

### `grpc_api.error_mode`

Type: `string`.

No documentation available.

### `grpc_api.address`

Type: `string`.

No documentation available.

### `grpc_api.port`

Type: `int`. Default: `10000`.

No documentation available.

### `grpc_api.key`

Type: `string`.

No documentation available.

### `grpc_api.tls`

Type: `TLSConfig` object.

No documentation available.

#### `grpc_api.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

#### `grpc_api.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

#### `grpc_api.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

#### `grpc_api.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

#### `grpc_api.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

#### `grpc_api.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

#### `grpc_api.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

### `grpc_api.reflection`

Type: `bool`.

No documentation available.

### `grpc_api.max_receive_message_size`

Type: `int`.

No documentation available.

## `consumers`

Type: `[]Consumer` object. Default: `[]`.

`consumers` is a configuration for message queue consumers. For example, Centrifugo can consume
messages from PostgreSQL transactional outbox table, or from Kafka topics.

### `consumers[].name`

Type: `string`.

`name` is a unique name required for each consumer.

### `consumers[].enabled`

Type: `bool`.

`enabled` must be true to tell Centrifugo to run configured consumer.

### `consumers[].type`

Type: `string`.

`type` describes the type of consumer.

### `consumers[].postgresql`

Type: `PostgresConsumerConfig` object.

`postgresql` allows defining options for consumer of postgresql type.

#### `consumers[].postgresql.dsn`

Type: `string`.

No documentation available.

#### `consumers[].postgresql.outbox_table_name`

Type: `string`.

No documentation available.

#### `consumers[].postgresql.num_partitions`

Type: `int`. Default: `1`.

No documentation available.

#### `consumers[].postgresql.partition_select_limit`

Type: `int`. Default: `100`.

No documentation available.

#### `consumers[].postgresql.partition_poll_interval`

Type: `Duration`. Default: `300ms`.

No documentation available.

#### `consumers[].postgresql.partition_notification_channel`

Type: `string`.

No documentation available.

#### `consumers[].postgresql.tls`

Type: `TLSConfig` object.

No documentation available.

##### `consumers[].postgresql.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `consumers[].postgresql.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `consumers[].postgresql.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `consumers[].postgresql.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `consumers[].postgresql.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `consumers[].postgresql.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `consumers[].postgresql.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

### `consumers[].kafka`

Type: `KafkaConsumerConfig` object.

`kafka` allows defining options for consumer of kafka type.

#### `consumers[].kafka.brokers`

Type: `[]string`.

No documentation available.

#### `consumers[].kafka.topics`

Type: `[]string`.

No documentation available.

#### `consumers[].kafka.consumer_group`

Type: `string`.

No documentation available.

#### `consumers[].kafka.max_poll_records`

Type: `int`. Default: `100`.

No documentation available.

#### `consumers[].kafka.tls`

Type: `TLSConfig` object.

`tls` for the connection to Kafka.

##### `consumers[].kafka.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

##### `consumers[].kafka.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

##### `consumers[].kafka.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

##### `consumers[].kafka.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

##### `consumers[].kafka.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

##### `consumers[].kafka.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

##### `consumers[].kafka.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

#### `consumers[].kafka.sasl_mechanism`

Type: `string`.

`sasl_mechanism` when not empty enables SASL auth.

#### `consumers[].kafka.sasl_user`

Type: `string`.

No documentation available.

#### `consumers[].kafka.sasl_password`

Type: `string`.

No documentation available.

#### `consumers[].kafka.partition_buffer_size`

Type: `int`. Default: `16`.

`partition_buffer_size` is the size of the buffer for each partition consumer.
This is the number of records that can be buffered before the consumer
will pause fetching records from Kafka. By default, this is 16.

#### `consumers[].kafka.fetch_max_bytes`

Type: `int32`.

`fetch_max_bytes` is the maximum number of bytes to fetch from Kafka in a single request.
If not set the default 50MB is used.

#### `consumers[].kafka.publication_data_mode`

Type: `KafkaPublicationDataModeConfig` object.

`publication_data_mode` is a configuration for the mode where message payload already
contains data ready to publish into channels, instead of API command.

##### `consumers[].kafka.publication_data_mode.enabled`

Type: `bool`.

`enabled` enables Kafka publication data mode for the Kafka consumer.

##### `consumers[].kafka.publication_data_mode.channels_header`

Type: `string`.

`channels_header` is a header name to extract channels to publish data into
(channels must be comma-separated). Ex. of value: "channel1,channel2".

##### `consumers[].kafka.publication_data_mode.idempotency_key_header`

Type: `string`.

`idempotency_key_header` is a header name to extract Publication idempotency key from
Kafka message. See https://centrifugal.dev/docs/server/server_api#publishrequest.

##### `consumers[].kafka.publication_data_mode.delta_header`

Type: `string`.

`delta_header` is a header name to extract Publication delta flag from Kafka message
which tells Centrifugo whether to use delta compression for message or not.
See https://centrifugal.dev/docs/server/delta_compression and
https://centrifugal.dev/docs/server/server_api#publishrequest.

## `websocket`

Type: `WebSocket` object.

`websocket` configuration. This transport is enabled by default.

### `websocket.disabled`

Type: `bool`.

No documentation available.

### `websocket.handler_prefix`

Type: `string`. Default: `/connection/websocket`.

No documentation available.

### `websocket.compression`

Type: `bool`.

No documentation available.

### `websocket.compression_min_size`

Type: `int`.

No documentation available.

### `websocket.compression_level`

Type: `int`. Default: `1`.

No documentation available.

### `websocket.read_buffer_size`

Type: `int`.

No documentation available.

### `websocket.use_write_buffer_pool`

Type: `bool`.

No documentation available.

### `websocket.write_buffer_size`

Type: `int`.

No documentation available.

### `websocket.write_timeout`

Type: `Duration`. Default: `1000ms`.

No documentation available.

### `websocket.message_size_limit`

Type: `int`. Default: `65536`.

No documentation available.

## `sse`

Type: `SSE` object.

`sse` is a configuration for Server-Sent Events based bidirectional emulation transport.

### `sse.enabled`

Type: `bool`.

No documentation available.

### `sse.handler_prefix`

Type: `string`. Default: `/connection/sse`.

No documentation available.

### `sse.max_request_body_size`

Type: `int`. Default: `65536`.

No documentation available.

## `http_stream`

Type: `HTTPStream` object.

`http_stream` is a configuration for HTTP streaming based bidirectional emulation transport.

### `http_stream.enabled`

Type: `bool`.

No documentation available.

### `http_stream.handler_prefix`

Type: `string`. Default: `/connection/http_stream`.

No documentation available.

### `http_stream.max_request_body_size`

Type: `int`. Default: `65536`.

No documentation available.

## `webtransport`

Type: `WebTransport` object.

`webtransport` is a configuration for WebTransport transport. EXPERIMENTAL.

### `webtransport.enabled`

Type: `bool`.

No documentation available.

### `webtransport.handler_prefix`

Type: `string`. Default: `/connection/webtransport`.

No documentation available.

### `webtransport.message_size_limit`

Type: `int`. Default: `65536`.

No documentation available.

## `uni_sse`

Type: `UniSSE` object.

`uni_sse` is a configuration for unidirectional Server-Sent Events transport.

### `uni_sse.enabled`

Type: `bool`.

No documentation available.

### `uni_sse.handler_prefix`

Type: `string`. Default: `/connection/uni_sse`.

No documentation available.

### `uni_sse.max_request_body_size`

Type: `int`. Default: `65536`.

No documentation available.

### `uni_sse.connect_code_to_http_response`

Type: `ConnectCodeToHTTPResponse` object.

No documentation available.

#### `uni_sse.connect_code_to_http_response.enabled`

Type: `bool`.

No documentation available.

#### `uni_sse.connect_code_to_http_response.transforms`

Type: `[]ConnectCodeToHTTPResponseTransform` object. Default: `[]`.

No documentation available.

##### `uni_sse.connect_code_to_http_response.transforms[].code`

Type: `uint32`.

No documentation available.

##### `uni_sse.connect_code_to_http_response.transforms[].to`

Type: `TransformedConnectErrorHttpResponse` object.

No documentation available.

##### `uni_sse.connect_code_to_http_response.transforms[].to.status_code`

Type: `int`.

No documentation available.

##### `uni_sse.connect_code_to_http_response.transforms[].to.body`

Type: `string`.

No documentation available.

## `uni_http_stream`

Type: `UniHTTPStream` object.

`uni_http_stream` is a configuration for unidirectional HTTP streaming transport.

### `uni_http_stream.enabled`

Type: `bool`.

No documentation available.

### `uni_http_stream.handler_prefix`

Type: `string`. Default: `/connection/uni_http_stream`.

No documentation available.

### `uni_http_stream.max_request_body_size`

Type: `int`. Default: `65536`.

No documentation available.

### `uni_http_stream.connect_code_to_http_response`

Type: `ConnectCodeToHTTPResponse` object.

No documentation available.

#### `uni_http_stream.connect_code_to_http_response.enabled`

Type: `bool`.

No documentation available.

#### `uni_http_stream.connect_code_to_http_response.transforms`

Type: `[]ConnectCodeToHTTPResponseTransform` object. Default: `[]`.

No documentation available.

##### `uni_http_stream.connect_code_to_http_response.transforms[].code`

Type: `uint32`.

No documentation available.

##### `uni_http_stream.connect_code_to_http_response.transforms[].to`

Type: `TransformedConnectErrorHttpResponse` object.

No documentation available.

##### `uni_http_stream.connect_code_to_http_response.transforms[].to.status_code`

Type: `int`.

No documentation available.

##### `uni_http_stream.connect_code_to_http_response.transforms[].to.body`

Type: `string`.

No documentation available.

## `uni_websocket`

Type: `UniWebSocket` object.

`uni_websocket` is a configuration for unidirectional WebSocket transport.

### `uni_websocket.enabled`

Type: `bool`.

No documentation available.

### `uni_websocket.handler_prefix`

Type: `string`. Default: `/connection/uni_websocket`.

No documentation available.

### `uni_websocket.compression`

Type: `bool`.

No documentation available.

### `uni_websocket.compression_min_size`

Type: `int`.

No documentation available.

### `uni_websocket.compression_level`

Type: `int`. Default: `1`.

No documentation available.

### `uni_websocket.read_buffer_size`

Type: `int`.

No documentation available.

### `uni_websocket.use_write_buffer_pool`

Type: `bool`.

No documentation available.

### `uni_websocket.write_buffer_size`

Type: `int`.

No documentation available.

### `uni_websocket.write_timeout`

Type: `Duration`. Default: `1000ms`.

No documentation available.

### `uni_websocket.message_size_limit`

Type: `int`. Default: `65536`.

No documentation available.

### `uni_websocket.join_push_messages`

Type: `bool`.

`join_push_messages` when enabled allow uni_websocket transport to join messages together into
one frame using Centrifugal client protocol delimiters: new line for JSON protocol and
length-prefixed format for Protobuf protocol. This can be useful to reduce system call
overhead when sending many small messages. The client side must be ready to handle such
joined messages coming in one WebSocket frame.

## `uni_grpc`

Type: `UniGRPC` object.

`uni_grpc` is a configuration for unidirectional gRPC transport.

### `uni_grpc.enabled`

Type: `bool`.

No documentation available.

### `uni_grpc.address`

Type: `string`.

No documentation available.

### `uni_grpc.port`

Type: `int`. Default: `11000`.

No documentation available.

### `uni_grpc.max_receive_message_size`

Type: `int`.

No documentation available.

### `uni_grpc.tls`

Type: `TLSConfig` object.

No documentation available.

#### `uni_grpc.tls.enabled`

Type: `bool`.

`enabled` turns on using TLS.

#### `uni_grpc.tls.cert_pem`

Type: `PEMData`.

`cert_pem` is a PEM certificate.

#### `uni_grpc.tls.key_pem`

Type: `PEMData`.

`key_pem` is a path to a file with key in PEM format.

#### `uni_grpc.tls.server_ca_pem`

Type: `PEMData`.

`server_ca_pem` is a server root CA certificate in PEM format.
The client uses this certificate to verify the server's certificate during the TLS handshake.

#### `uni_grpc.tls.client_ca_pem`

Type: `PEMData`.

`client_ca_pem` is a client CA certificate in PEM format.
The server uses this certificate to verify the client's certificate during the TLS handshake.

#### `uni_grpc.tls.insecure_skip_verify`

Type: `bool`.

`insecure_skip_verify` turns off server certificate verification.

#### `uni_grpc.tls.server_name`

Type: `string`.

`server_name` is used to verify the hostname on the returned certificates.

## `emulation`

Type: `Emulation` object.

`emulation` endpoint is enabled automatically when at least one bidirectional emulation transport
is configured (SSE or HTTP Stream).

### `emulation.handler_prefix`

Type: `string`. Default: `/emulation`.

No documentation available.

### `emulation.max_request_body_size`

Type: `int`. Default: `65536`.

No documentation available.

## `admin`

Type: `Admin` object.

`admin` web UI configuration.

### `admin.enabled`

Type: `bool`.

No documentation available.

### `admin.handler_prefix`

Type: `string`.

No documentation available.

### `admin.password`

Type: `string`.

`password` is an admin password.

### `admin.secret`

Type: `string`.

`secret` is a secret to generate auth token for admin requests.

### `admin.insecure`

Type: `bool`.

`insecure` turns on insecure mode for admin endpoints - no auth
required to connect to web interface and for requests to admin API.
Admin resources must be protected by firewall rules in production when
this option enabled otherwise everyone from internet can make admin
actions.

### `admin.web_path`

Type: `string`.

`web_path` is path to admin web application to serve.

### `admin.web_proxy_address`

Type: `string`.

`web_proxy_address` is an address for proxying to the running admin web application app.
So it's possible to run web app in dev mode and point Centrifugo to its address for
development purposes.

### `admin.external`

Type: `bool`.

`external` is a flag to run admin interface on external port.

## `prometheus`

Type: `Prometheus` object.

`prometheus` metrics configuration.

### `prometheus.enabled`

Type: `bool`.

No documentation available.

### `prometheus.handler_prefix`

Type: `string`. Default: `/metrics`.

No documentation available.

### `prometheus.instrument_http_handlers`

Type: `bool`.

No documentation available.

## `health`

Type: `Health` object.

`health` check endpoint configuration.

### `health.enabled`

Type: `bool`.

No documentation available.

### `health.handler_prefix`

Type: `string`. Default: `/health`.

No documentation available.

## `swagger`

Type: `Swagger` object.

`swagger` documentation (for server HTTP API) configuration.

### `swagger.enabled`

Type: `bool`.

No documentation available.

### `swagger.handler_prefix`

Type: `string`. Default: `/swagger`.

No documentation available.

## `debug`

Type: `Debug` object.

`debug` helps to enable Go profiling endpoints.

### `debug.enabled`

Type: `bool`.

No documentation available.

### `debug.handler_prefix`

Type: `string`. Default: `/debug/pprof`.

No documentation available.

## `opentelemetry`

Type: `OpenTelemetry` object.

`opentelemetry` is a configuration for OpenTelemetry tracing.

### `opentelemetry.enabled`

Type: `bool`.

No documentation available.

### `opentelemetry.api`

Type: `bool`.

No documentation available.

### `opentelemetry.consuming`

Type: `bool`.

No documentation available.

## `graphite`

Type: `Graphite` object.

`graphite` is a configuration for export metrics to Graphite.

### `graphite.enabled`

Type: `bool`.

No documentation available.

### `graphite.host`

Type: `string`. Default: `localhost`.

No documentation available.

### `graphite.port`

Type: `int`. Default: `2003`.

No documentation available.

### `graphite.prefix`

Type: `string`. Default: `centrifugo`.

No documentation available.

### `graphite.interval`

Type: `Duration`. Default: `10s`.

No documentation available.

### `graphite.tags`

Type: `bool`.

No documentation available.

## `usage_stats`

Type: `UsageStats` object.

`usage_stats` is a configuration for usage stats sending.

### `usage_stats.disabled`

Type: `bool`.

No documentation available.

## `node`

Type: `Node` object.

`node` is a configuration for Centrifugo Node as part of cluster.

### `node.name`

Type: `string`.

`name` is a human-readable name of Centrifugo node in cluster. This must be unique for each running node
in a cluster. By default, Centrifugo constructs name from the hostname and port. Name is shown in admin web
interface. For communication between nodes in a cluster, Centrifugo uses another identifier – unique ID
generated on node start, so node name plays just a human-readable identifier role.

### `node.info_metrics_aggregate_interval`

Type: `Duration`. Default: `60s`.

`info_metrics_aggregate_interval` is a time interval to aggregate node info metrics.

## `shutdown`

Type: `Shutdown` object.

`shutdown` is a configuration for graceful shutdown.

### `shutdown.timeout`

Type: `Duration`. Default: `30s`.

No documentation available.

## `pid_file`

Type: `string`.

`pid_file` is a path to write a file with Centrifugo process PID.

## `enable_unreleased_features`

Type: `bool`.

`enable_unreleased_features` enables unreleased features. These features are not stable and may be removed even
in minor release update. Evaluate and share feedback if you find some feature useful and want it to be stabilized.

