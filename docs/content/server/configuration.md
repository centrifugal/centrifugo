# Configuration

Centrifugo expects JSON, TOML or YAML as format of configuration file. Thanks to brilliant Go library for application configuration - [viper](https://github.com/spf13/viper).

But first let's inspect all available command-line options:

```bash
centrifugo -h
```

You should see something like this as output:

```
Centrifugo – real-time messaging server

Usage:
   [flags]
   [command]

Available Commands:
  checkconfig Check configuration file
  genconfig   Generate simple configuration file to start with
  help        Help about any command
  version     Centrifugo version number

Flags:
  -a, --address string             interface address to listen on
      --admin                      enable admin web interface
      --admin_insecure             use insecure admin mode – no auth required for admin socket
      --api_insecure               use insecure API mode
      --client_insecure            start in insecure client mode
  -c, --config string              path to config file (default "config.json")
      --debug                      enable debug endpoints
  -e, --engine string              engine to use: memory or redis (default "memory")
      --grpc_api                   enable GRPC API server
      --grpc_client                enable GRPC client server
  -h, --help                       help for this command
      --internal_port string       custom port for internal endpoints
      --log_file string            optional log file - if not specified logs go to STDOUT
      --log_level string           set the log level: debug, info, error, fatal or none (default "info")
  -n, --name string                unique node name
      --pid_file string            optional path to create PID file
  -p, --port string                port to bind HTTP server to (default "8000")
      --prometheus                 enable Prometheus metrics endpoint
      --redis_db string            Redis database (Redis engine) (default "0")
      --redis_host string          Redis host (Redis engine) (default "127.0.0.1")
      --redis_master_name string   name of Redis master Sentinel monitors (Redis engine)
      --redis_password string      Redis auth password (Redis engine)
      --redis_pool int             Redis pool size (Redis engine) (default 256)
      --redis_port string          Redis port (Redis engine) (default "6379")
      --redis_sentinels string     comma-separated list of Sentinel addresses (Redis engine)
      --redis_url string           Redis connection URL in format redis://:password@hostname:port/db (Redis engine)
      --tls                        enable TLS, requires an X509 certificate and a key file
      --tls_cert string            path to an X509 certificate file
      --tls_key string             path to an X509 certificate key

Use " [command] --help" for more information about a command.
```

### version

To show version and exit run:

```
centrifugo version
```

### JSON file

Centrifugo requires configuration file on start. As was mentioned earlier it must be a file with valid JSON.

This is a minimal Centrifugo configuration file:

```javascript
{
  "secret": "secret"
}
```

The only field that is required is **secret**. Secret used to create HMAC signs. Keep it strong and in secret!

### TOML file

Centrifugo also supports TOML format for configuration file:

```
centrifugo --config=config.toml
```

Where `config.toml` contains:

```
secret = "secret"
log_level = "debug"
```

I.e. the same configuration as JSON file above. We will talk about what is `namespaces` field soon.

### YAML file

And YAML config also supported. `config.yaml`:

```
secret: secret
log_level: debug
```

With YAML remember to use spaces, not tabs when writing configuration file.

### checkconfig command

Centrifugo has special command to check configuration file `checkconfig`:

```bash
centrifugo checkconfig --config=config.json
```

If any errors found during validation – program will exit with error message and exit code 1.

### genconfig command

Another command is `genconfig`:

```
centrifugo genconfig -c config.json
```

It will generate the minimal required configuration file automatically.

### Important options

Some of the most important options you can configure when running Centrifugo:

* `address` – bind your Centrifugo to specific interface address (by default `""`)
* `port` – port to bind Centrifugo to (by default `8000`)
* `engine` – engine to use - `memory` or `redis` (by default `memory`). Read more about engines in next sections.

Note that some options can be set via command-line. Command-line options are more valuable when set than configuration file's options. See description of [viper](https://github.com/spf13/viper) – to see more details about configuration options priority.

### Channel options

Let's look on options related to channels. Channel is an entity to which clients can subscribe to receive messages published into that channel. Channel is just a string (several symbols has special meaning in Centrifugo - see special chapter to find more information about channels). The following options will affect channel behaviour:

* `publish` – allow clients to publish messages into channels directly (from client side). Your application will never receive those messages. In idiomatic case all messages must be published by your application backend using Centrifugo API. But this option can be useful when you want to build something without backend-side validation and saving into database. This option can also be useful for demos and prototyping real-time ideas. Note that client can only publish data into channel after successfully subscribed on it. By default it's `false`.

* `anonymous` – this option enables anonymous access (with empty user ID in connection parameters). In most situations your application works with authorized users so every user has its own unique id. But if you provide real-time features for public access you may need unauthorized access to some channels. Turn on this option and use empty string as user ID. By default `false`.

* `presence` – enable/disable presence information. Presence is a structure with clients currently subscribed on channel. By default `false` – i.e. no presence information available for channels.

* `join_leave` – enable/disable sending join(leave) messages when client subscribes on channel (unsubscribes from channel). By default `false`.

* `history_size` – history size (amount of messages) for channels. As Centrifugo keeps all history messages in memory it's very important to limit maximum amount of messages in channel history to reasonable minimum. By default history size is `0` - this means that channels will have no history messages at all. As soon as history enabled then `history_size` defines maximum amount of messages that Centrifugo will keep for **each** channel in namespace during history lifetime (see below).

* `history_lifetime` – interval in seconds how long to keep channel history messages. As all history is storing in memory it is also very important to get rid of old history data for unused (inactive for a long time) channels. By default history lifetime is `0` – this means that channels will have no history messages at all. **So to get history messages you should wisely configure both `history_size` and `history_lifetime` options**.

* `history_recover` (**new in v1.2.0**) – boolean option, when enabled Centrifugo will try to recover missed messages published while client was disconnected for some reason (bad internet connection for example). By default `false`. This option must be used in conjunction with reasonably configured message history for channel i.e. `history_size` and `history_lifetime` **must be set** (because Centrifugo uses channel message history to recover messages). Also note that note all real-time events require this feature turned on so think wisely when you need this. See more details about how this option works in [special chapter](recover.md).

* `history_drop_inactive` (**new in v1.3.0**) – boolean option, allows to drastically reduce resource usage (engine memory usage, messages travelling around) when you use message history for channels. In couple of words when enabled Centrifugo will drop history messages that no one needs. Please, see [issue on Github](https://github.com/centrifugal/centrifugo/issues/50) to get more information about option use case scenario and edge cases it involves.

Let's look how to set some of these options in config:

```javascript
{
    "secret": "my-secret-key",
    "anonymous": true,
    "publish": true,
    "presence": true,
    "join_leave": true,
    "history_size": 10,
    "history_lifetime": 30,
    "history_recover": true
}
```

And the last channel specific option is `namespaces`. `namespaces` are optional and if set must be an array of namespace objects. Namespace allows to configure custom options for channels starting with namespace name. This provides a great control over channel behaviour.

Namespace has a name and the same channel options (with same defaults) as described above.

* `name` - unique namespace name (name must must consist of letters, numbers, underscores or hyphens and be more than 2 symbols length i.e. satisfy regexp `^[-a-zA-Z0-9_]{2,}$`).

If you want to use namespace options for channel - you must include namespace name into
channel name with `:` as separator:

`public:messages`

`gossips:messages`

Where `public` and `gossips` are namespace names from project `namespaces`.

All things together here is an example of `config.json` which includes registered project with all options set and 2 additional namespaces in it:

```javascript
{
    "secret": "very-long-secret-key",
    "anonymous": true,
    "publish": true,
    "presence": true,
    "join_leave": true,
    "history_size": 10,
    "history_lifetime": 30,
    "namespaces": [
        {
          "name": "public",
          "publish": true,
          "anonymous": true,
          "history_size": 10,
          "history_lifetime": 30,
          "history_recover": true
        },
        {
          "name": "gossips",
          "presence": true,
          "join_leave": true
        }
    ]
}
```

Channel `news` will use global project options.

Channel `public:news` will use `public` namespace's options.

Channel `gossips:news` will use `gossips` namespace's options.

### Advanced configuration

Centrifugo has some options for which default values make sense for most applications. In many case you
don't need (and you really should not) change them. This chapter is about such options.

#### client_channel_limit

Default: 128

Sets maximum number of different channel subscriptions single client can have.

#### channel_max_length

Default: 255

Sets maximum length of channel name.

#### channel_user_connection_limit

Default: 0

Maximum number of connections from user (with known user ID) to Centrifugo node. By default - unlimited.

#### client_request_max_size

Default: 65536

Maximum allowed size of request from client in bytes.

#### client_queue_max_size

Default: 10485760

Maximum client message queue size in bytes to close slow reader connections. By default - 10mb.

#### sockjs_heartbeat_delay

Default: 25

Interval in seconds how often to send SockJS h-frames to client.

#### websocket_compression

Default: false

Enable websocket compression, see special chapter in docs.

#### gomaxprocs

Default: 0

By default Centrifugo runs on all available CPU cores. If you want to limit amount of cores Centrifugo can utilize in one moment use this option.

### Advanced endpoint configuration.

After you started Centrifugo you have several endpoints available. As soon as you have not provided any extra options you have 3 endpoints by default.

#### Default endpoints.

First is SockJS endpoint - it's needed to serve client connections that use SockJS library:

```
http://localhost:8000/connection/sockjs
```

Next is raw Websocket endpoint to serve client connections that use pure Websocket protocol:

```
ws://localhost:8000/connection/websocket
```

And finally you have API endpoint to `publish` messages to channels (and execute other available API commands):

```
http://localhost:8000/api
```

By default all endpoints work on port `8000`. You can change it using `port` option:

```
{
    "port": 9000
}
```

In production setup you will have your domain name in endpoint addresses above instead of `localhost`. Also if your Centrifugo will be behind proxy or load balancer software you most probably won't have ports in your endpoint addresses. What will always be the same as shown above are URL paths: `/connection/sockjs`, `/connection/websocket`, `/api`.

Let's look at possibilities to tweak available endpoints.

#### Admin endpoints.

First is enabling admin endpoints:

```
{
    ...
    "admin": true,
    "admin_password": "password",
    "admin_secret": "secret"
}
```

This makes the following endpoint available:

```
ws://localhost:8000
```

At this address you will see embedded admin web interface. You can log into it using `admin_password` value shown above.

#### Debug endpoints.

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

– will show you useful info about internal state of Centrifugo instance. This info is especially helpful when troubleshooting.

#### Custom admin and API ports

We strongly recommend to not expose admin, debug and API endpoints to Internet. In case of admin endpoint this step provides extra protection to `/`, `/admin/auth`, `/admin/api` endpoints, debug endpoints. Protecting API endpoint will allow you to use `api_insecure` mode to omit passing API key in each request.

So it's a good practice to protect admin and API endpoints with firewall. For example you can do this in `location` section of Nginx configuration.

Though sometimes you don't have access to per-location configuration in your proxy/load balancer software. For example
when using Amazon ELB. In this case you can change ports on which your admin and API endpoints work.

To run admin endpoints on custom port use `admin_port` option:

```
{
    ...
    "admin_port": 10000
}
```

So admin web interface will work on address:
 
```
ws://localhost:10000
```

Also debug page will be available on new custom admin port too:

```
http://localhost:10000/debug/pprof/
```

To run API server on it's own port use `api_port` option:

```
{
    ...
    "api_port": 10001
}
```

Now you should send API requests to:

```
http://localhost:10001/api
```
