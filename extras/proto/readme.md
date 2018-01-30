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
GOGO=1 gomplate -f $GOPATH/src/github.com/centrifugal/centrifugo/extras/proto/api.template.proto > $GOPATH/src/github.com/centrifugal/centrifugo/lib/proto/api/api.proto
```

### Client ptoto

Generate for cross-language client usage:

```
gomplate -f client.template.proto
```

Generate for internal Centrifugo usage.

```
GOGO=1 gomplate -f $GOPATH/src/github.com/centrifugal/centrifugo/extras/proto/client.template.proto > $GOPATH/src/github.com/centrifugal/centrifugo/lib/proto/client.proto
```
