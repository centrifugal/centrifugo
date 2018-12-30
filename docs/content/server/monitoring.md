# Monitoring

Centrifugo supports reporting metrics in Prometheus format and can automatically export metrics to Graphite.

### Prometheus

To enable Prometheus endpoint start Centrifugo with `prometheus` option on:

```json
{
    ...
    "prometheus": true
}
```

```
./centrifugo --config=config.json
```

This will enable `/metrics` endpoint so Centrifugo instance can be monitored by your Prometheus server.

### Graphite

To enable automatic export to Graphite (via TCP):

```json
{
    "graphite": true,
    "graphite_host": "localhost",
    "graphite_port": 2003
}
```

By default stats will be aggregated over 10 seconds interval inside Centrifugo and then pushed to Graphite over TCP connection.

If you need to change this aggregation interval use `graphite_interval` option (in seconds, default `10`). This option available since v2.1.0
