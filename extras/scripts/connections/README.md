Connections script
==================

Create N subscribed to M channels (on channel `channel1`, `channel2`...`channelM`) clients and keep them connected until interrupted.

run with 4000 connected subscribed clients and one channel:

```bash
go run connections.go ws://localhost:8000/connection/websocket secret 4000
```

with 500 connected subscribed clients each subscribed on 40 channels:

```bash
go run connections.go ws://localhost:8000/connection/websocket secret 500 40
```
