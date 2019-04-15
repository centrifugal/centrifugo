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
