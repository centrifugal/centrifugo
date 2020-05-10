# How message recovery works

This description uses `offset` field available since Centrifugo v2.5.0 which replaced two `uint32` fields `seq` and `gen` in client protocol schema. This means that here we describe a case when Centrifugo config contains `v3_use_offset` option enabled:

```json
{
  ...
  "v3_use_offset": true
}
```

!!! note
    For `seq` and `gen` recovery works in similar way, but we have two `uint32` fields instead of single `uint64` `offset`.

One of the most interesting features of Centrifugo is message recovery after short network disconnects. This mechanism allows client to automatically get missed publications on successful resubscribe to channel after being disconnected for a while. In general, you would query your application backend for actual state on every client reconnect - but message recovery feature allows Centrifugo to deal with this and restore missed publications from history cache thus radically reducing load on your application backend and your main database in some scenarios.

To enable recovery mechanism for channels set `history_recover` boolean configuration option to `true` on the configuration file top-level or for a channel namespace.

When subscribing on channels Centrifugo will return missed `publications` to client in subscribe `Reply`, also it will return special `recovered` boolean flag to indicate whether all missed publications successfully recovered after disconnect or not.

Centrifugo recovery model based on two fields in protocol: `offset` and `epoch`. All fields managed automatically by Centrifugo client libraries, but it's good to know how recovery works under the hood.

Once `history_recover` option enabled every publication will have incremental (inside channel) `offset` field. This field has `uint64` type.

Another field is string `epoch`. It exists to handle cases when history storage has been restarted while client was in disconnected state so publication numeration in a channel started from scratch. For example at moment Memory engine does not persist publication sequences on disk so every restart will start numeration from scratch, after each restart new `epoch` field generated, and we can understand in recovery process that client could miss messages thus returning correct `recovered` flag in subscribe `Reply`. This also applies to Redis engine – if you do not use AOF with fsync then sequences can be lost after Redis restart. When using Redis engine you need to use fully in-memory model strategy or AOF with fsync to guarantee reliability of `recovered` flag sent by Centrifugo.

When server receives subscribe command with boolean flag `recover` set to `true` and `offset`, `epoch` set to values last seen by a client (see `SubscribeRequest` type in [protocol definitions](https://github.com/centrifugal/protocol/blob/master/definitions/client.proto)) it can try to find all missed publications from history cache. Recovered publications will be passed to client in subscribe `Reply` in correct order, and your publication handler will be automatically called to process each missed message.

You can also manually implement your own recovery algorithm on top of basic PUB/SUB possibilities that Centrifugo provides. As we said above you can simply ask your backend for an actual state after every client reconnect completely bypassing recovery mechanism described here.
