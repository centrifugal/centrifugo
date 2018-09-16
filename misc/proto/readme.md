Install `gomplate`:

```
go get github.com/hairyhenderson/gomplate
```

### API proto

Generate for cross-language API client usage:

```
gomplate -f $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/api.template.proto > $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/api.proto
```

Generate for internal Centrifuge library usage.

```
GOGO=1 gomplate -f $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/api.template.proto > $GOPATH/src/github.com/centrifugal/centrifuge/internal/proto/apiproto/api.proto
```

### Client proto

Generate for cross-language client usage:

```
gomplate -f $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/client.template.proto > $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/client.proto
```

Generate for internal Centrifuge library usage.

```
GOGO=1 gomplate -f $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/client.template.proto > $GOPATH/src/github.com/centrifugal/centrifuge/internal/proto/client.proto
```

### API examples

For Python:

```python
import grpc
import api_pb2_grpc as api_grpc
import api_pb2 as api_pb

channel = grpc.insecure_channel('localhost:8001')
stub = api_grpc.CentrifugeStub(channel)

try:
    resp = stub.Info(api_pb.InfoRequest())
except grpc.RpcError as err:
    # GRPC level error.
    print(err.code(), err.details())
else:
    if resp.error.code:
        # Centrifuge server level error.
        print(resp.error.code, resp.error.message)
    else:
        print(resp.result)
```
