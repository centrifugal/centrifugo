# Signal handling

You can send HUP signal to Centrifugo to reload configuration:

```
kill -HUP <PID>
```

Though at moment this will only reload channel and namespace configuration.

Also Centrifugo tries to gracefully shutdown client connections when SIGINT or SIGTERM signals received.
