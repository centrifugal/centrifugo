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
