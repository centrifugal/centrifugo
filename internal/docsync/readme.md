# Document sync framework

Centrifugo can sync documents between server and clients. This is a powerful feature that simplifies building 
real-time applications without thinking much about data structures and missed updates recovery on the client side.

## Document sync protocol

Each document has unique ID. 

When client subscribes to a document it receives current document state and then all changes to this document.
When client unsubscribes from a document it stops receiving updates for this document.

Each document is JSON.

Document sync protocol is based on Centrifugo channel subscriptions. Each document has its own channel. 
When client subscribes to a document it subscribes to a channel with the same name as document ID. 
When client unsubscribes from a document it unsubscribes from a channel with the same name as document ID.

## Why Centrifugo document sync approach?

In the document sync approach of Centrifugo we aim to not add many rules on how you structure, keep and update your
documents. We want to be language-agnostic and allow you to use any backend to store your documents. But at the same
time we want to provide a simple and efficient way to sync documents between server and clients solving many common
problems like synchronizing initial state, dealing with missed updates, sending only document deltas over the wire.

We continue following the idea of Centrifugo - let backend be a source of truth and let Centrifugo be a transport layer.
But in the document sync approach we add a bit more - we use Centrifugo to efficiently calculate document diffs and
only send these diffs to clients. This allows to minimize traffic and make document sync approach efficient.

## How it works

When client subscribes to a document it receives current document state 
in subscription success event.

Here is how it looks in Javascript:

```javascript
const centrifuge = new Centrifuge('ws://localhost:8000/connection/websocket')
centrifuge.connect()
const document = centrifuge.newDocument('document:1')
document.subscribe()
```

Instead of updates to documents, on publication will be called with the entire document state. Centrifugo sends
only diffs to clients (actually, depends on document size), SDK internally applies these diffs to the document state.

Centrifugo receives subscription request, at this point Centrifugo does the following:

* Centrifugo checks local cache for the document
* if document is found in the cache then Centrifugo sends it to client.
* if document is not found in the cache then Centrifugo asks backend for the document and sends it to client (saving doc to cache before that).

> Centrifugo automatically batches documents **in one namespace** together when requesting the backend.

Let's say there is a change in document on the backend. Backend sends updated document to Centrifugo.

* Document reaches Centrifugo node
* Centrifugo saves document to local cache, then loads prev version of doc from Redis. Calculates diff between these 
two versions and publishes diff to all clients subscribed to this document. Centrifugo only does this in case new
version of document is greater than previous version. Publications additionally contain version of document
corresponding to published changes (`__version` in `tags`). If version is less - skip.
* If there is no previous state - Centrifugo sends the entire document to all clients subscribed to this document.

Why version is useful? This allows Centrifugo to detect stale data coming from the backend. For example, two clients 
from different nodes subscribed to the same channel – but while requests were inflight version incremented by one.
Centrifugo will save to local cache only the document with the highest version.

If version is not provided – Centrifugo will still work but will not be able to detect stale data, this may be suitable
for cases where having such checks is not important.

From Broker perspective: we still need a stream with deltas. This allows clients to catch up with missed updates.
Document cache may be in-memory or backed by Redis. We can start with in-memory cache and then add Redis support.

Need to make sure we have a proper sync of initial state load with real-time updates. This is important to avoid
situations when client receives initial state and then missed updates that were already included in initial state.

## Alternative approach

Let's think in a different way. Upon publish we send entire doc to Redis. Document is saved to stream 
(let's say we only keep 1 last document in this case). But when sending to PUB/SUB we send previous and current
document versions. This allows making a diff on Centrifugo node after getting from PUB/SUB and send only diff to
local clients. This approach allows to avoid keeping document cache in memory on Centrifugo nodes, we don't need
to think about cache eviction, missing previous version.

From the client perspective, we still need to send the entire document on subscription success. The idea here
is to load last document from Redis and send it to client. For example, add `state` field to subscription success
event. If there is no document in Redis - two options:

* not send anything to client, next update will send the entire document since there is no previous version in Redis.
* load document from the backend and send it to client.

Do we need versions in this case?

Let's consider edge cases.

* Client receives initial state, the data in Redis lost. Centrifuge detects position issue, client resubscribes and
then client will receive recovered: false, or document will be loaded from backend and put into history stream
(without publishing). The question is still there - should we cache the document in Redis (or maybe locally?), or not?
* Client receives initial state, the disconnects for a long time. Then client reconnects and we see that it has offset
100, while document has offset 107. And history contains entire documents! It seems that this mode requires keeping
only one document in history stream. Then we don't care about offsets and just send current document to client
(with delta: false).

So we need to introduce `delta` flag to publication. This flag will be true if we send only diff, false if we send 
entire document. This flag will be used by client to understand if it should apply diff to the document or just
replace the entire document with the received one.

Note, in this case backend MUST publish entire document to Centrifugo.

This feature may be called "state channels", or "document channels". Or better - state channels when there is no
integration with the backend and document channels when there is integration with the backend.

So:

* history keeps entire state.
* backend publishes entire state to Centrifugo.
* Centrifugo calculates diff and sends it to clients.

What if client just subscribed? It MAY receive state from Redis in `state` field of subscription success event. Or,
if there is no state in Redis - it MAY receive state from the backend. So we need to provide field in protocol and
hook to set this field upon subscription.

Setting state MUST be synchronized with real-time updates. State must be versioned to resolve conflicts/races.

It seems we must design features separate. First design document channel with keeping only last document in history.
Then add delta support.

Let's start with document channel

## Document channel

Document channel is a channel where we send entire document on each publication.

When client subscribes to a document channel it receives current document state in subscription success event.
In `state` field.

Then it receives all the following document updates in real-time.

Document has version, we track version of document sent to each client. We can drop updates with version less than
version of document sent to client.

When document is updated on the backend it publishes entire document to Centrifugo.
If possible - attaches version to it.

Centrifugo receives document, saves it to history stream, then sends it to all clients subscribed to this document.

If there is no previous document Centrifuge provides a hook to load it from the backend. When document is loaded
it is sent to a channel. This way we automatically cache it and client receives it almost immediately.

## Example on client-side and configuration

```javascript
const centrifuge = new Centrifuge('ws://localhost:8000/connection/websocket')
centrifuge.connect()
const subscription = centrifuge.newSubscription('documents:document:1')
subscription.on('publication', (ctx) => {
    console.log(ctx.data)
})
subscription.subscribe()
```

Centrifugo configuration:

```json
{
  "namespaces": [
    {
      "name": "documents",
      "history_size": 1,
      "history_ttl": "30m",
      "force_recovery": true,
      "document": true
    }
  ]
}
```

With delta calculation:

```json
{
  "namespaces": [
    {
      "name": "documents",
      "history_size": 1,
      "history_ttl": "30m",
      "force_recovery": true,
      "document": true,
      "delta": "jsonpatch"
    }
  ]
}
```

What will come to a client in the subscribe response on first connect?

Always a document/state field with the entire document no matter client requested. In `publications`. If it's not
possible to load document then `recovered` will be false. Otherwise - true.

If client provides epoch/offset then we can avoid sending the entire document in `publications` to client on reconnect.

What will come to the client in subscribe response on successful recovery?

Same as on first connect.

What will come to client in subscribe response on unsuccessful recovery?

In this mode unsuccessful recovery is possible when no data in Redis and no document proxy configured. If error on
calling Redis - internal error. If document proxy configured but not able to load document then internal error will
be returned too.

If no value in Redis history and we load it from the backend over document proxy - what to do with the returned
document? Should we put it into history stream? Or just send to client?

It seems we should put it into history stream. This way we will have a consistent way to recover missed updates.
But we don't need to send it to PUB/SUB to not have duplicate documents on subscribing client side.

When recovering client will receive the document from the history stream. And it will be sent in `publications` field?

Yes. It will be sent in `publications` field.

What about always sending in `publications`? This way we will have a consistent way to handle initial state and
recovered state. Also, we will send tags due that.

Yes, it seems we should always send in `publications` field. This way we will have a consistent way to handle initial
state and recovered state. Also, we will send tags due that and no new protocol field needed.

What if client provides epoch/offset, and we have document in Redis, but we understand it has same offset?

This means client has some cached document. We can then return `recovered: true` and skip sending document
in `publications`.

What if client provides epoch/offset, and we have document in Redis, but we understand it has same offset + 1?

This means client has some cached document. We can then return `recovered: true` and send new document
in `publications`.

What if client provides epoch/offset, and we have document in Redis, but we understand it has same offset + 2?

This means client has some cached document. We can then return `recovered: true` and send only latest document.

What if client provides epoch/offset, and we have document in Redis, but we understand it has same offset - 1 or
different epoch?

This means client has some cached document, but it's outdated. We can then return `recovered: true` and send new
document in `publications`.
