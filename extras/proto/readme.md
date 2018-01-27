Install `gomplate`:

```
go get github.com/hairyhenderson/gomplate
```

Generate for cross-language API client usage:

```
gomplate -f api.template.proto
```


Generate for internal Centrifugo usage.

```
GOGO=1 gomplate -f api.template.proto
```
