## Example of creating notifications trigger

```
CREATE OR REPLACE FUNCTION centrifugo_notify_partition_change()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify('centrifugo_partition_change', NEW.partition::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER centrifugo_notify_partition_trigger
AFTER INSERT ON chat_outbox
FOR EACH ROW
EXECUTE FUNCTION centrifugo_notify_partition_change();
```

## How to configure Debezium

```
{
    "name": "centrifugo-connector",
    "config": {
        "connector.class": "io.debezium.connector.postgresql.PostgresConnector",
        "database.hostname": "localhost",
        "database.port": "5432",
        "database.user": "grandchat",
        "database.password": "grandchat",
        "database.dbname": "grandchat",
        "database.server.name": "db",
        "table.include.list": "public.chat_outbox",
        "database.history.kafka.bootstrap.servers": "kafka:9092",
        "database.history.kafka.topic": "schema-changes.chat_outbox",
        "plugin.name": "pgoutput",
        "tasks.max": "1",
        "topic.creation.default.cleanup.policy": "delete",
        "topic.creation.default.partitions": "1",
        "topic.creation.default.replication.factor": "1",
        "topic.creation.default.retention.ms": "604800000",
        "topic.creation.enable": "true",
        "topic.prefix": "postgres",
        "key.converter": "org.apache.kafka.connect.json.JsonConverter",
        "value.converter": "org.apache.kafka.connect.json.JsonConverter",
        "key.converter.schemas.enable": "false",
        "value.converter.schemas.enable": "false",
        "poll.interval.ms": "100",
        "transforms": "extractContent",
        "transforms.extractContent.type": "org.apache.kafka.connect.transforms.ExtractField$Value",
        "transforms.extractContent.field": "after",
        "message.key.columns": "public.chat_outbox:partition",
        "snapshot.mode": "never"
    }
}
```
