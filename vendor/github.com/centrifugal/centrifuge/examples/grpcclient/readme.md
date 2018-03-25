Connect to Centrifuge GRPC client server over GRPC.

This is a very naive implementaion just for proof of concept at moment.

Without TLS:

```
go run main.go -addr localhost:8001 -channel chat:index
```

With TLS:

```
go run main.go -addr localhost:443 -channel chat:index -tls
```
