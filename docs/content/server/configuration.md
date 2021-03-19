# Configuration

Here we will look at how Centrifugo can be configured.

## Configuration ways

Centrifugo can be configured in several ways:

* over command-line flags, see `centrifugo -h` for available flags, command-line flags limited to most frequently used
* over configuration file, configuration file supports all options mentioned in this documentation
* over OS environment variables, **all Centrifugo options can be set over env in format `CENTRIFUGO_<OPTION_NAME>`** (mostly straightforward except namespaces - [see how to set namespaces via env](channels.md#setting-namespaces-over-env))

The basic way to start with Centrifugo is run `centrifugo genconfig` command which will generate `config.json` configuration file with some options (in a current directory), so you can then run Centrifugo:

```
centrifugo -c config.json
```

Below while describing configuration file format we will look at the meaning of the required options.

## Config file formats

Centrifugo supports different configuration file formats. 

### JSON config format

Centrifugo requires configuration file on start. As was mentioned earlier it must be a file with valid JSON.

This is a minimal Centrifugo configuration file:

```json
{
  "v3_use_offset": true,
  "token_hmac_secret_key": "<YOUR-SECRET-STRING-HERE>",
  "api_key": "<YOUR-API-KEY-HERE>",
}
```

The only two fields required are **token_hmac_secret_key** and **api_key**.

!!!note
    To be fair latest Centrifugo releases introduced a new way of authenticating connections over [proxy HTTP request](proxy.md#connect-proxy) from Centrifugo to application backend, and a way to publish messages to channels over [proxy request to backend](proxy.md#publish-proxy). Also there is GRPC server API that can be used instead of HTTP API – so `api_key` not used there. This means that in some setups both `token_hmac_secret_key` and `api_key` are not required at all. But here we describe the traditional way of running Centrifugo - with JWT authentication and publishing messages over server HTTP API.

`token_hmac_secret_key` used to check JWT signature (more about JWT in [authentication chapter](authentication.md)). API key used for Centrifugo API endpoint authorization, see more in [chapter about server HTTP API](http_api.md). Keep both values in secret and never reveal to clients.

The option `v3_use_offset` turns on using latest client-server protocol `offset` field (**will be used by default in Centrifugo v3 so better to use it from start**).

### TOML config format

Centrifugo also supports TOML format for configuration file:

```
centrifugo --config=config.toml
```

Where `config.toml` contains:

```
v3_use_offset = true
token_hmac_secret_key = "<YOUR-SECRET-STRING-HERE>"
api_key = "<YOUR-API-KEY-HERE>"
log_level = "debug"
```

I.e. the same configuration as JSON file above with one extra option to define logging level.

### YAML config format

YAML config also supported. `config.yaml`:

```
v3_use_offset: true
token_hmac_secret_key: "<YOUR-SECRET-STRING-HERE>"
api_key: "<YOUR-API-KEY-HERE>"
log_level: debug
```

With YAML remember to use spaces, not tabs when writing configuration file.

## Important options

Some of the most important options you can configure when running Centrifugo:

* `address` – bind your Centrifugo to specific interface address (by default `""`)
* `port` – port to bind Centrifugo to (by default `8000`)
* `engine` – engine to use - `memory` or `redis` (by default `memory`). Read more about engines in [special chapter](engines.md).

Note that some options can be set via command-line. Command-line options are more valuable when set than configuration file's options. See description of [viper](https://github.com/spf13/viper) – to see more details about configuration options priority.

## Advanced options

Centrifugo has some options for which default values make sense for most applications. In many case you don't need (and you really should not) change them. This chapter is about such options.

### client_channel_limit

Default: 128

Sets maximum number of different channel subscriptions single client can have.

### channel_max_length

Default: 255

Sets maximum length of channel name.

### client_user_connection_limit

Default: 0

Maximum number of connections from user (with known user ID) to Centrifugo node. By default, unlimited.

The important thing to emphasize is that `client_user_connection_limit` works only per one Centrifugo node and exists mostly to protect Centrifugo from many connections from a single user – but not for business logic limitations. This means that if you will scale nodes – say run 10 Centrifugo nodes – then a user will be able to create 10 connections (one to each node).

### client_request_max_size

Default: 65536

Maximum allowed size of request from client in bytes.

### client_queue_max_size

Default: 10485760

Maximum client message queue size in bytes to close slow reader connections. By default - 10mb.

### client_anonymous

Default: false

Enable a mode when all clients can connect to Centrifugo without JWT connection token. In this case all connections without token will be treated as anonymous (i.e. with empty user ID) and only can subscribe to channels with `anonymous` option enabled.

### client_concurrency

Available since Centrifugo v2.8.0

Default: 0

`client_concurrency` when set tells Centrifugo that commands from client must be processed concurrently.

By default, concurrency disabled – Centrifugo processes commands received from a client one by one. This means that if a client issues two RPC requests to a server then Centrifugo will process the first one, then the second one. If the first RPC call is slow then the client will wait for the second RPC response much longer than it could (even if second RPC is very fast). If you set `client_concurrency` to some value greater than 1 then commands will be processed concurrently (in parallel) in separate goroutines (with maximum concurrency level capped by `client_concurrency` value). Thus, this option can effectively reduce the latency of individual requests. Since separate goroutines involved in processing this mode adds some performance and memory overhead – though it should be pretty negligible in most cases. This option applies to all commands from a client (including subscribe, publish, presence, etc).

### sockjs_heartbeat_delay

Default: 25

Interval in seconds how often to send SockJS h-frames to client.

### websocket_compression

Default: false

Enable websocket compression, see chapter about websocket transport for more details.

### gomaxprocs

Default: 0

By default, Centrifugo runs on all available CPU cores. If you want to limit amount of cores Centrifugo can utilize in one moment use this option.

## Advanced endpoint configuration.

After you started Centrifugo you have several endpoints available. As soon as you have not provided any extra options you have 3 endpoints by default.

### Default endpoints.

The main endpoint is raw Websocket endpoint to serve client connections that use pure Websocket protocol:

```
ws://localhost:8000/connection/websocket
```

Then there is SockJS endpoint - it's needed to serve client connections that use SockJS library:

```
http://localhost:8000/connection/sockjs
```

And finally you have API endpoint to `publish` messages to channels (and execute other available API commands):

```
http://localhost:8000/api
```

By default, all endpoints work on port `8000`. You can change it using `port` option:

```
{
    "port": 9000
}
```

In production setup you will have your domain name in endpoint addresses above instead of `localhost`. Also if your Centrifugo will be behind proxy or load balancer software you most probably won't have ports in your endpoint addresses. What will always be the same as shown above are URL paths: `/connection/sockjs`, `/connection/websocket`, `/api`.

Let's look at possibilities to tweak available endpoints.

### Admin endpoints.

First is enabling admin endpoints:

```
{
    ...
    "admin": true,
    "admin_password": "password",
    "admin_secret": "secret"
}
```

This makes the following endpoint available: http://localhost:8000

At this address you will see admin web interface. You can log into it using `admin_password` value shown above.

### Debug endpoints.

Next, when Centrifugo started in debug mode some extra debug endpoints become available. To start in debug mode add `debug` option to config:

```
{
    ...
    "debug": true
}
```

And endpoint:

```
http://localhost:8000/debug/pprof/
```

– will show you useful info about internal state of Centrifugo instance. This info is especially helpful when troubleshooting. See [wiki page](https://github.com/centrifugal/centrifugo/wiki/Investigating-performance-issues) for more info.

### Health check endpoint

New in v2.1.0

Use `health` boolean option (by default `false`) to enable healthcheck endpoint which will be available on path `/health`. Also available over command-line flag:

```bash
./centrifugo -c config.json --health
```

### Custom internal ports

We strongly recommend to not expose API, admin, debug and prometheus endpoints to Internet. The following Centrifugo endpoints are considered internal:

* API endpoint (`/api`) - for HTTP API requests
* Admin web interface endpoints (`/`, `/admin/auth`, `/admin/api`) - used by web interface
* Prometheus endpoint (`/metrics`) - used for exposing server metrics in Prometheus format 
* Health check endpoint (`/health`) - used to do health checks
* Debug endpoints (`/debug/pprof`) - used to inspect internal server state

It's a good practice to protect those endpoints with firewall. For example, you can do this in `location` section of Nginx configuration.

Though sometimes you don't have access to per-location configuration in your proxy/load balancer software. For example when using Amazon ELB. In this case you can change ports on which your internal endpoints work.

To run internal endpoints on custom port use `internal_port` option:

```
{
    ...
    "internal_port": 9000
}
```

So admin web interface will work on address:
 
```
http://localhost:9000
```

Also debug page will be available on new custom port too:

```
http://localhost:9000/debug/pprof/
```

The same for API and prometheus endpoint.

### Disable default endpoints

These options available since v2.4.0

To disable websocket endpoint set `websocket_disable` boolean option to `true`.

To disable SockJS endpoint set `sockjs_disable` boolean option to `true`.

To disable API endpoint set `api_disable` boolean option to `true`.

### Customize handler endpoints

Starting from Centrifugo v2.2.5 it's possible to customize server HTTP handler endpoints. To do this Centrifugo supports several options:

* `admin_handler_prefix` (default `""`) - to control Admin panel URL prefix
* `websocket_handler_prefix` (default `"/connection/websocket"`) - to control WebSocket URL prefix
* `sockjs_handler_prefix` (default `"/connection/sockjs"`) - to control SockJS URL prefix
* `api_handler_prefix` (default `"/api"`) - to control HTTP API URL prefix
* `prometheus_handler_prefix` (default `"/metrics"`) - to control Prometheus URL prefix
* `health_handler_prefix` (default `"/health"`) - to control health check URL prefix

## Signal handling

You can send HUP signal to Centrifugo to reload a configuration:

```
kill -HUP <PID>
```

Though at moment **this will only reload token secrets and channel options (top-level and namespaces)**.

Centrifugo tries to gracefully shutdown client connections when SIGINT or SIGTERM signals received. By default, maximum graceful shutdown period is 30 seconds but can be changed using `shutdown_timeout` (integer, in seconds) configuration option.
