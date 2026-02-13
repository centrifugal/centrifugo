Produce messages to Kafka topic to quickly test out Centrifugo Kafka consumer.

Run Centrifugo with:

```json
{
  "consumers": [
    {
      "name": "kafka_test",
      "enabled": true,
      "type": "kafka",
      "kafka": {
        "brokers": ["localhost:29092"],
        "topics": ["centrifugo"],
        "consumer_group": "centrifugo"
      }
    }
  ]
}
```

Use `docker-compose.yml` of Centrifugo:

```
docker compose up kafka
```

And:

```
go run ./misc/kafka_producer/
```
