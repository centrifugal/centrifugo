[![Join the chat at https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ](https://img.shields.io/badge/Telegram-Group-orange?style=flat&logo=telegram)](https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ) &nbsp;&nbsp;[![Join the chat at https://discord.gg/tYgADKx](https://img.shields.io/discord/719186998686122046?style=flat&label=Discord&logo=discord)](https://discord.gg/tYgADKx)

Centrifugo is a scalable real-time messaging server in language-agnostic way. Centrifugo works in conjunction with application backend written in any programming language. It runs as separate service and keeps persistent Websocket or SockJS connections from application clients (from web browsers or other environments like iOS/Android apps). When you need to deliver an event to your clients in real-time you publish it to Centrifugo API and Centrifugo then broadcasts event to all connected clients interested in this event (i.e. clients subscribed on event channel). In other words – this is a user-facing PUB/SUB server.

For more information follow to [Centrifugo documentation site](https://centrifugal.github.io/centrifugo/).

![scheme](https://raw.githubusercontent.com/centrifugal/centrifugo/master/docs/content/images/scheme_sketch.png)

### How to install

See [installation instructions](https://centrifugal.github.io/centrifugo/server/install/) in Centrifugo documentation.

### Demo

Try our [demo instance](https://centrifugo2.herokuapp.com/) on Heroku (admin password is `password`, token_hmac_secret_key is `secret`, API key is `api_key`). Or deploy your own Centrifugo instance in one click:

[![Deploy](https://www.herokucdn.com/deploy/button.svg)](https://heroku.com/deploy?template=https://github.com/centrifugal/centrifugo)

### Highlights

* Centrifugo is fast and capable to scale to millions of simultaneous connections
* Simple integration with any application – works as separate service, provides HTTP and GRPC API
* Client-side libraries for popular frontend environments – for both web and mobile development
* JSON and binary Protobuf Websocket client protocol based on strict Protobuf schema
* SockJS polyfill for web browsers without Websocket support
* Unidirectional transport support without need in client libraries - use native APIs (SSE, Fetch, WebSocket, GRPC)
* User authentication with JWT or over connection request proxy to configured HTTP endpoint
* Proper connection management and expiration control
* Various types of channels: private, user-limited
* Various types of subscriptions: client-side or server-side
* Transform RPC calls over WebSocket/SockJS to configured HTTP endpoint call
* Presence information for channels (show all active clients in channel)
* History information for channels (last messages published into channel)
* Join/leave events for channels (client goes online/offline)
* Automatic recovery of missed messages between client reconnects over configured retention period
* Built-in administrative web panel
* Cross platform – works on Linux, MacOS and Windows
* Ready to deploy (Docker, RPM/DEB packages, automatic TLS certificates, Prometheus instrumentation, Grafana dashboard)
* Open-source license
