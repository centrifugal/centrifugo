# envconfig

This is a fork of https://github.com/kelseyhightower/envconfig, original license left unchanged.

First reason is this issue: https://github.com/kelseyhightower/envconfig/issues/148.

Basically, we are not using ALT names here, so the main change is:

```
name := strings.ToUpper(ftype.Tag.Get("envconfig"))
if name == "" {
    name = ftype.Name
}

// Capture information about the config variable
info := varInfo{
    Name:  name,
    Field: f,
    Tags:  ftype.Tag,
    //Alt:   strings.ToUpper(ftype.Tag.Get("envconfig")),
}
```

The second reason of fork is that we use exported `VarInfo` here instead of `varInfo`. This helps Centrifugo to find unknown configuration options.
