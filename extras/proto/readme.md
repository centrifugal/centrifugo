Install `gomplate`:

```
go get github.com/hairyhenderson/gomplate
```

### API proto

Generate for cross-language API client usage:

```
gomplate -f api.template.proto
```

Generate for internal Centrifugo usage.

```
GOGO=1 gomplate -f $GOPATH/src/github.com/centrifugal/centrifugo/extras/proto/api.template.proto > $GOPATH/src/github.com/centrifugal/centrifugo/lib/proto/apiproto/api.proto
```

### Client proto

Generate for cross-language client usage:

```
gomplate -f client.template.proto
```

Generate for internal Centrifugo usage.

```
GOGO=1 gomplate -f $GOPATH/src/github.com/centrifugal/centrifugo/extras/proto/client.template.proto > $GOPATH/src/github.com/centrifugal/centrifugo/lib/proto/client.proto
```
