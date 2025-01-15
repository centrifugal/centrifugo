Create local Sentinel based Redis setup consisting of 1 Sentinel and 3 Redis nodes (master, 2 replicas). Note, in production you need at least 3 Sentinels for high availability.

## Run

```
bash start.sh
```

## Emulate failover

Connect to Sentinel:

```
redis-cli -p 26379
```

And run:

```
sentinel failover mymaster
```
