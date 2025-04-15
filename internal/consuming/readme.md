### Nats JetsStream

```
docker compose up nats_jestream
```

Then:

```
nats stream add TEST --subjects "test" --storage memory
```

Config:

```json
{
  "consumers": [
    {
      "enabled": true,
      "name": "mynats",
      "type": "nats_jetstream",
      "nats_jetstream": {
        "url": "nats://localhost:4222",
        "stream_name": "TEST",
        "subjects": ["test"],
        "durable_consumer_name": "centrifugo"
      }
    }
  ]
}
```

Publish:

```
nats pub test '{"method": "publish", "payload": {"channel": "test", "data": {"input": "test"}}}'
```


### Redis stream

Config:

```json
{
  "consumers": [
    {
      "enabled": true,
      "name": "myredis",
      "type": "redis_stream",
      "redis_stream": {
        "address": "redis://localhost:6379",
        "streams": [
          "test"
        ],
        "consumer_group": "centrifugo"
      }
    }
  ]
}
```

Publish:

```
XADD test * method publish payload '{"data": {"input": "streamssss"}, "channel": "chat:index"}'
```
