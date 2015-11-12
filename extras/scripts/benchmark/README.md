Benchmark script
================

Subscribe clients on channel `test` and then publish message into this channel. Measure time until
all messages received by subscribed clients.

To run benchmark.go allow `publish` into channel and execute:

```json
{
  "secret": "secret",
  "publish": true
}

```

run with 4000 max clients incrementing client amount by 1000 and repeat measuremrents 50 times:

```bash
go run benchmark.go ws://localhost:8000/connection/websocket secret 4000 1000 50
```
