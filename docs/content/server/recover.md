# How message recovery works

One of the most important features of Centrifugo is message recovery after short network disconnects. This is often the case when we are moving around - for example sudden tunnel can result in internet connection lost. Or when we switching networks on our device. Message recovery feature is especially useful when dealing with Centrifugo node restart - many client connection disconnect and then reconnect at once - if users subscribed on channels with important data it's a good practice to restore missed state on successful reconnect. In general you would query your application backend for actual state on every network/shutdown reconnect - but message recovery feature allows Centrifugo itself to deal with this and restore missed messages from history cache thus reducing load on your application backend during reconnects.

When subscribing on channels Centrifugo will return missed `publications` to client and also special `recovered` boolean flag to indicate whether all messages were recovered after disconnect or not.

To enable recovery mechanism for channels set `history_recover` boolean configuration option to `true` on configuration top level or for channel namespace.

Centrifugo recovery model based on two fields in protocol: `last` and `away`. Both fields are managed automatically by Centrifugo client libraries but it's good to know how recovery works under the hood.

Every time client receives a new publication from channel client library remembers `uid` of received `Publication`. When resubscribing to channel after reconnect client passes last seen publication `uid` in subscribe command.

Client also passes `away` field when resubscribing which is a number of seconds client was in unsubscribed state. This is a number of seconds rounded to the upper value. Client also adds timeout values in seconds to this interval to compensate possible network latencies when communicating with server.

When server receives subscribe request with `last` field set it can look at history cache and find all next publications (following one with provided `last` uid). If `last` field not provided (for example there were no new messages in channel for a long time) then server will only rely on `away` field. In this case server will set `recovered` flag to `true` only if client was away (absent) for an interval of time that is not larger than history lifetime and amount of messages in history cache less than configured history size.

Message recovery in Centrifugo is not a 100% bulletproof scheme (as it has some assumptions regarding to time) but it should work for significant amount of real life cases without message loss. If you need more reliability in channel message recovery then you can manually implement your own recovery algorithm on top of basic PUB/SUB possibilities that Centrifugo provides.

And you can always simply ask your backend for an actual state after client reconnect completely bypassing recovery mechanism described here.

### Recovery on example

Consider channel `chat_messages` client subscribed to.

First let's imagine a situation when client receives new message from this channel. Let the `uid` value of that message be a `ZZaavc`.


And then client disconnects because of lost internet connection.

When connection lost client saves current time for each subscription. This saved time will be used to calculate `away` field value when client will do resubscription attempt.

Let's suppose client will be 5 seconds in disconnected state and then connection will be restored and client will resubscribe on channel.

It will pass two fields in subscribe command:

* `last` - in this case `ZZaavc`
* `away` - in this case `5` + client request timeout. I.e `10` in case of client request timeout of `5 seconds`.

If server finds publication with `ZZaavc` uid in history it can be sure that all following messages are all messages client lost during reconnect (and will set `recovered` flag to `true`). 

If there were no publication with uid `ZZaavc` in history then `recovered` flag will be set to true only if provided `away` value is less than channel `history_lifetime` and history currently contains amount of publications strictly less than `history_size`. Actually server adds one more second to `away` value to compensate rounding errors.

This means that if `history_size` is `10` and `history_lifetime` is `60` then client must reconnect in 0-49 seconds to be sure all missed publications were successfully recovered.
