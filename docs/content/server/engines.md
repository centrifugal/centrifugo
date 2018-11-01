# Engines

* [Memory engine](#memory-engine)
* [Redis engine](#redis-engine)

Engine in Centrifugo is responsible for publishing messages between nodes, handle PUB/SUB broker subscriptions, save/retrieve presence and history data.

By default Centrifugo uses Memory engine. There is also Redis engine available.

The difference between them - with Memory engine you can start only one node of Centrifugo, while Redis engine allows to run several nodes on different machines and they will be connected via Redis, will know about each other due to Redis and will also keep history and presence data in Redis instead of Centrifugo node process memory so this data can be accessed from each node.

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

### Memory engine

Supports only one node. Nice choice to start with. Supports all features keeping everything in Centrifugo node process memory. You don't need to install Redis when using this engine.

Advantages:

* fast
* does not require separate Redis setup

Disadvantages:

* does not allow to scale nodes (actually you still can scale Centrifugo with Memory engine but you have to publish data into each Centrifugo node and you won't have consistent state of presence)

### Redis engine

Allows scaling Centrifugo nodes to different machines. Nodes will use Redis as message broker. Redis engine keeps presence and history data in Redis, uses Redis PUB/SUB for internal nodes communication.

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

Some of these options can be set over command-line arguments (see `centrifugo -h` output), some only over configuration file.

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
