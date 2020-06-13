# Engines

* [Memory engine](#memory-engine)
* [Redis engine](#redis-engine)

Engine in Centrifugo is responsible for publishing messages between nodes, handle PUB/SUB broker subscriptions, save/retrieve presence and history data.

By default, Centrifugo uses Memory engine. There is also Redis engine available.

The difference between them - with Memory engine you can start only one node of Centrifugo, while Redis engine allows running several nodes on different machines and they will be connected via Redis, will know about each other due to Redis and will also keep history and presence data in Redis instead of Centrifugo node process memory so this data can be accessed from each node.

To set engine you can use `engine` configuration option. Available values are `memory` and `redis`. Default value is `memory`.

For example to work with Redis engine:

```
centrifugo --config=config.json --engine=redis
```

Or just set `engine` in config:

```json
{
    ...
    "engine": "redis"
}
```

## Memory engine

Supports only one node. Nice choice to start with. Supports all features keeping everything in Centrifugo node process memory. You don't need to install Redis when using this engine.

Advantages:

* fast
* does not require separate Redis setup

Disadvantages:

* does not allow scaling nodes (actually you still can scale Centrifugo with Memory engine but you have to publish data into each Centrifugo node and you won't have consistent state of presence)
* does not persist message history in channels between Centrifugo restarts

Several configuration options related to Memory engine:

* `memory_history_meta_ttl` (int, default `0`) - sets a time in seconds of history stream metadata expiration. Stream metadata is an information about current offset number in channel and epoch value. By default, metadata for channels does not expire. Though in some cases – when channels created for а short time and then not used anymore – created metadata can stay in memory while not actually useful. For example, you can have a personal user channel but after using your app for a while user left it forever. In long-term perspective this can be an unwanted memory leak. Setting a reasonable value to this option (usually much bigger than history retention period) can help. In this case unused channel metadata will eventually expire. Available since v2.5.0

## Redis engine

Allows scaling Centrifugo nodes to different machines. Nodes will use Redis as message broker. Redis engine keeps presence and history data in Redis, uses Redis PUB/SUB for internal node communication.

**Minimal Redis version is 3.2.0**

Several configuration options related to Redis engine:

* `redis_host` (string, default `"127.0.0.1"`) - Redis server host
* `redis_port` (int, default `6379`) - Redis server port
* `redis_url` (string, default `""`) - optional Redis connection URL
* `redis_password` (string, default `""`) - Redis password
* `redis_db` (int, default `0`) - number of Redis db to use
* `redis_tls` (boolean, default `false`) - enable Redis TLS connection (new in v2.0.2)
* `redis_tls_skip_verify` (boolean, default `false`) - disable Redis TLS host verification (new in v2.0.2)
* `redis_sentinels` (string, default `""`) - comma separated list of Sentinels for HA
* `redis_master_name` (string, default `""`) - name of Redis master Sentinel monitors
* `redis_prefix` (string, default `"centrifugo"`) – custom prefix to use for channels and keys in Redis
* `redis_history_meta_ttl` (int, default `0`) - sets a time in seconds of history stream metadata expiration in Redis Engine. Meta key in Redis is a HASH that contains current offset number in channel and epoch value. By default, metadata for channels does not expire. Though in some cases – when channels created for а short time and then not used anymore – created stream metadata can stay in memory while not actually useful. For example, you can have a personal user channel but after using your app for a while user left it forever. In long-term perspective this can be an unwanted memory leak. Setting a reasonable value to this option (usually much bigger than history retention period) can help. In this case unused channel metadata will eventually expire. Available since v2.3.0
* `redis_streams` (boolean, default `false`) – turns on using Redis Streams instead of List data structure for keeping history

All of these options can be set over configuration file. Some of them can be set over command-line arguments (see `centrifugo -h` output).

Let's describe a bit more `redis_url` option. `redis_url` allows to set Redis connection parameters in a form of URL in format `redis://:password@hostname:port/db_number`. When `redis_url` set Centrifugo will use URL instead of values provided in `redis_host`, `redis_port`, `redis_password`, `redis_db` options.

### Scaling with Redis tutorial

Let's see how to start several Centrifugo nodes using Redis engine. We will start 3 Centrifugo nodes and all those nodes will be connected via Redis.

First, you should have Redis running. As soon as it's running - we can launch 3 Centrifugo instances. Open your terminal and start first one:

```
centrifugo --config=config.json --port=8000 --engine=redis --redis_host=127.0.0.1 --redis_port=6379
```

If your Redis on the same machine and runs on its default port you can omit `redis_host` and `redis_port` options in command above.

Then open another terminal and start another Centrifugo instance:

```
centrifugo --config=config.json --port=8001 --engine=redis --redis_host=127.0.0.1 --redis_port=6379
```

Note that we use another port number (`8001`) as port 8000 already busy by our first Centrifugo instance. If you are starting Centrifugo instances on different machines then you most probably can use
the same port number (`8000` or whatever you want) for all instances.

And finally let's start third instance:

```
centrifugo --config=config.json --port=8002 --engine=redis --redis_host=127.0.0.1 --redis_port=6379
```

Now you have 3 Centrifugo instances running on ports 8000, 8001, 8002 and clients can connect to any of them. You can also send API requests to any of those nodes – as all nodes connected over Redis PUB/SUB message will be delivered to all interested clients on all nodes.

To load balance clients between nodes you can use Nginx – you can find its configuration here in documentation.

### Redis Sentinel for high availability

Centrifugo supports official way to add high availability to Redis - Redis [Sentinel](http://redis.io/topics/sentinel).

For this you only need to utilize 2 Redis Engine options: `redis_master_name` and `redis_sentinels`.

`redis_master_name` - is a name of master your Sentinels monitor.

`redis_sentinels` - comma-separated addresses of Sentinel servers. At least one known server required.

So you can start Centrifugo which will use Sentinels to discover redis master instance like this:

```
centrifugo --config=config.json --engine=redis --redis_master_name=mymaster --redis_sentinels=":26379"
```

Sentinel configuration files can look like this:

```
port 26379
sentinel monitor mymaster 127.0.0.1 6379 2
sentinel down-after-milliseconds mymaster 10000
sentinel failover-timeout mymaster 60000
```

You can find how to properly setup Sentinels [in official documentation](http://redis.io/topics/sentinel).

Note that when your redis master instance down there will be small downtime interval until Sentinels
discover a problem and come to quorum decision about new master. The length of this period depends on
Sentinel configuration.

### Haproxy instead of Sentinel configuration

Alternatively you can use Haproxy between Centrifugo and Redis to let it properly balance traffic to Redis master. In this case you still need to configure Sentinels but you can omit Sentinel specifics from Centrifugo configuration and just use Redis address as in simple non-HA case.

For example you can use something like this in Haproxy config:

```
listen redis
    server redis-01 127.0.0.1:6380 check port 6380 check inter 2s weight 1 inter 2s downinter 5s rise 10 fall 2
    server redis-02 127.0.0.1:6381 check port 6381 check inter 2s weight 1 inter 2s downinter 5s rise 10 fall 2 backup
    bind *:16379
    mode tcp
    option tcpka
    option tcplog
    option tcp-check
    tcp-check send PING\r\n
    tcp-check expect string +PONG
    tcp-check send info\ replication\r\n
    tcp-check expect string role:master
    tcp-check send QUIT\r\n
    tcp-check expect string +OK
    balance roundrobin
```

And then just point Centrifugo to this Haproxy:

```
centrifugo --config=config.json --engine=redis --redis_host=localhost --redis_port=16379
```

### Redis sharding

Centrifugo has a built-in Redis sharding support.

This resolves situation when Redis becoming a bottleneck on large Centrifugo setup. Redis is single-threaded server, it's very fast but it's power is not infinite so when your Redis approaches 100% CPU usage then sharding feature can help your application to scale.

At moment Centrifugo supports simple comma-based approach to configuring Redis shards. Let's just look on examples.

To start Centrifugo with 2 Redis shards on localhost running on port 6379 and port 6380:

```
centrifugo --config=config.json --engine=redis --redis_port=6379,6380
```

To start Centrifugo with Redis instances on different hosts:

```
centrifugo --config=config.json --engine=redis --redis_host=192.168.1.34,192.168.1.35
```

If you also need to customize AUTH password, Redis DB number then you can use `redis_url` option.

Note, that due to how Redis PUB/SUB work it's not possible (and it's pretty useless anyway) to run shards in one Redis instances using different Redis DB numbers.

When sharding enabled Centrifugo will spread channels and history/presence keys over configured Redis instances using consistent hashing algorithm. At moment we use Jump consistent hash algorithm (see [paper](https://arxiv.org/pdf/1406.2294.pdf) and [implementation](https://github.com/dgryski/go-jump))

### Redis cluster

Redis cluster supported since Centrifugo v2.5.0

Running Centrifugo with Redis cluster is simple and can be achieved using `redis_cluster_addrs` option. This is an array of strings. Each element of array is a comma-separated Redis cluster seed nodes. For example:

```json
{
    ...
    "redis_cluster_addrs": [
        "localhost:30001,localhost:30002,localhost:30003"
    ]
}
```

Actually you don't need to list all Redis cluster nodes in config – only several working nodes is enough to start.

To set the same over environment variable:

```bash
CENTRIFUGO_REDIS_CLUSTER_ADDRS="localhost:30001" CENTRIFUGO_ENGINE=redis ./centrifugo
```

If you need to shard data between several Redis clusters then simply add one more string with seed nodes of another cluster to this array:

```json
{
    ...
    "redis_cluster_addrs": [
        "localhost:30001,localhost:30002,localhost:30003",
        "localhost:30101,localhost:30102,localhost:30103"
    ]
}
```

Sharding between different Redis clusters can make sense due to the fact how PUB/SUB works in Redis cluster. It does not scale linearly when adding nodes as all PUB/SUB messages got copied to every cluster node. See [this discussion](https://github.com/antirez/redis/issues/2672) for more information on topic. To spread data between different Redis clusters Centrifugo uses the same consistent hashing algorithm described above (i.e. `Jump`).

To reproduce the same over environment variable use `space` to separate different clusters:

```bash
CENTRIFUGO_REDIS_CLUSTER_ADDRS="localhost:30001,localhost:30002 localhost:30101,localhost:30102" CENTRIFUGO_ENGINE=redis ./centrifugo
```

### Redis Streams

Since Centrifugo v2.5.0 it's possible to use [Redis Stream](https://redis.io/topics/streams-intro) data structure to keep Publication history inside channels.

Redis streams can help to reduce number of memory allocations Centrifugo does during message recovery process upon client reconnect inside large history streams. This can be critical for massive Centrifugo deployments that utilize recovery feature.

To turn on using Redis streams use boolean option `redis_streams`, default `false`.

Redis Streams can become default data structure to keep Publication history in Centrifugo v3.

### KeyDB

Centrifugo Redis engine seamlessly works with [KeyDB](https://keydb.dev/). KeyDB server is compatible with Redis and provides several additional features beyond. 

Though we can't give any promises about compatibility with KeyDB in future Centrifugo releases - while KeyDB is fully compatible with Redis things should work fine. That's why we consider this as **EXPERIMENTAL** feature.

Use KeyDB instead of Redis only if you are really sure you need it. Nothing stops you from running several Redis instances per each core you have, configure sharding and obtain even better performance that KeyDB can provide (due to lack of synchronization between threads in Redis).

In order to run Centrifugo with KeyDB all you need to do is use `redis` engine option and run KeyDB server instead of Redis.
