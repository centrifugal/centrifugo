# envconfig

This is a copy of https://github.com/kelseyhightower/envconfig due to this issue: https://github.com/kelseyhightower/envconfig/issues/148.

Once there is a native way to disable Alt env vars we can switch back to the original package.

So the main change is:

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

In the code here.
