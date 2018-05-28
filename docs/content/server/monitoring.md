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

This will eneble `/metrics` endpoint so Centrifugo instance can be monitored by your Prometheus server.

### Graphite

To enable automatic export to Graphite (via TCP):

```json
{
    "graphite": true,
    "graphite_host": "localhost",
    "graphite_port": 2003
}
```
