# Redis

This section describes some aspects about deploying Redis for Centrifugo server.

### Sentinel for high availability

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

### Haproxy

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
