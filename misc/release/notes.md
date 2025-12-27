Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

This release **has a potential breaking change** in the Redis Cluster case where users try forming a Centrifugo cluster consisting of nodes with different Centrifugo versions (older than 6.6.0 and 6.6.0+). See details in the **Fixes** section below. We believe this scenario is very rare, and the risk of causing issues for anyone is minimal. The benefit is supporting ElastiCache Serverless Redis out of the box without extra options.

### Improvements

* We are actively working on a new Centrifugo Helm chart v13 major release. See [centrifugal/helm-charts#136](https://github.com/centrifugal/helm-charts/pull/136). The new chart will support new Kubernetes functionality, will have better docs, and more examples and tutorials for Google and AWS managed Kubernetes services. The plan is to release one more version of the v12 chart with Centrifugo v6.6.0 as the base `appVersion` and then move on to the v13 chart. Note that users of the v12 Helm chart will be able to use newer Centrifugo versions without any issues, as chart versioning is independent of Centrifugo versioning (done through the `appVersion`). However, new updates, fixes for chart functionality, and documentation will be released only for the v13 chart.
* Performance: optimizations for allocation efficiency under a batched message writing scenario and when publishing the message into channel.

### Fixes

* Fix slot issues when publishing with history to ElastiCache Serverless Redis. See the report in [#1087](https://github.com/centrifugal/centrifugo/issues/1087) and the fix in [centrifugal/centrifuge#541](https://github.com/centrifugal/centrifuge/pull/541). Centrifugo was not previously tested with ElastiCache Serverless Redis. The [finding](https://github.com/centrifugal/centrifugo/issues/1087#issuecomment-3667377731) that Serverless ElastiCache requires using hashtags in channels to follow the same slots as normal data structure keys goes beyond the Redis Cluster specification (where channels are not related to hash slots until using sharded PUB/SUB). The initial thought was to introduce a separate option to support this behavior, but after further consideration, Centrifugo will always use hashtags for channels in the Redis Cluster case. This means we have incompatible changes in the internal Redis protocol: nodes of different Centrifugo versions (<6.6.0 and 6.6.0+) will not work together in one cluster. This scenario is not common at all, and we have never encouraged such a setup. Moreover, when combined with the Redis Cluster condition (under 1% of setups), the chance of causing issues for someone is minimal. During rollout, message loss can happen, but only on the PUB/SUB layer, which is at-most-once anyway. History streams and recovery logic will work as usual and will prevent client state corruption.

### Miscellaneous

* This release is built with Go 1.25.5.
* Updated dependencies.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.6.0).

### Happy New Year

We would like to thank the Centrifugo community for staying with us throughout 2025. Your feedback, bug reports, discussions, and contributions help drive the project forward and make Centrifugo better with every release.

In 2025, we released Centrifugo v6 and, throughout the year, built important functionality on top of that foundation. Major highlights are: new async consumers from popular queue systems, server-side publication filtering, support for RFC 8441 (WebSocket over HTTP/2). A new real-time SDK for C# was introduced. Centrifugo reached 7k+ real installations, usage of our SDKs grew significantly according to CDN stats ([example](https://www.jsdelivr.com/package/npm/centrifuge?tab=stats)), and we saw more and more projects choosing Centrifugo as their real-time messaging solution. For the first time, we saw a local Go meetup where 2 out of 4 talks were about projects using Centrifugo. This is a great sign of growing adoption. We continue to receive consistently positive feedback from developers, highlighting Centrifugo as a reliable, efficient, and easy-to-use real-time messaging server.

We are looking forward to getting even more things done in 2026 and continuing to improve Centrifugo and the real-time messaging ecosystem.

Happy New Year to everyone, and best wishes for a successful year ahead! 🎄 May the Centrifugal force be with you! 🖲

— Centrifugal Labs team
