Several scripts to benchmark Redis engine performance.

1. fillqueue.go - fill one or more queues with publish API commands.

Usage:

```
go run fillqueue.go 10000 centrifugo.api
```

Or to fill many API queues:

```
go run fillqueue.go 10000 centrifugo.api.0 centrifugo.api.1 centrifugo.api.2
```

2. queueperf.go - look for one or more queues at print how many messages per seconds processed.

```
go run queueperf.go centrifugo.api.0 centrifugo.api.1
```

3. publish.go - publish as fast as possible into channels. Show how many published in second.

```
go run publish.go centrifugo.message. 10
```

will publish messages into 10 channels: centrifugo.message.0, centrifugo.message.1 ... centrifugo.message.9

4. receive.go - subscribe on channels and show how many processed in second

```
go run publish.go centrifugo.message. 10 2
```

will subscribe on 10 channels sharding them between 2 different subscribe connections.


