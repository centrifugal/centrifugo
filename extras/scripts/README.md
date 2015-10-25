To run benchmark.go allow `publish` into channel and execute:

```json
{
  "secret": "secret",
  "publish": true
}

```

And run:

```bash
go run benchmark.go ws://localhost:8000/connection/websocket secret 4000 1000 50
```
