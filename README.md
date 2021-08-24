[![Join the chat at https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ](https://img.shields.io/badge/Telegram-Group-orange?style=flat&logo=telegram)](https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ) &nbsp;&nbsp;[![Join the chat at https://discord.gg/tYgADKx](https://img.shields.io/discord/719186998686122046?style=flat&label=Discord&logo=discord)](https://discord.gg/tYgADKx)

Centrifugo is a scalable real-time messaging server in a language-agnostic way.

Centrifugo works in conjunction with application backend written in any programming language. It runs as a separate service and keeps persistent connections from application clients established over several supported types of transport (WebSocket, SockJS, EventSource, GRPC, HTTP-streaming). When you need to deliver an event to your clients in real-time, you publish it to Centrifugo server API – and Centrifugo then broadcasts the event to all connected clients interested in this event (clients subscribed to the event channel). In other words – Centrifugo is a user-facing PUB/SUB server.

For more information follow to [Centrifugo documentation site](https://centrifugal.dev).

![scheme](https://raw.githubusercontent.com/centrifugal/centrifugo/v2/docs/content/images/scheme_sketch.png)

### How to install

See [installation instructions](https://centrifugal.dev/docs/getting-started/installation) in Centrifugo documentation.

### Demo

Try our [demo instance](https://centrifugo3.herokuapp.com/) on Heroku (admin password is `password`, token_hmac_secret_key is `secret`, API key is `api_key`). Or deploy your own Centrifugo instance in one click:

[![Deploy](https://www.herokucdn.com/deploy/button.svg)](https://heroku.com/deploy?template=https://github.com/centrifugal/centrifugo)

### Highlights

* Centrifugo is fast and capable to scale to millions of simultaneous connections
* Simple integration with any application – works as separate service, provides HTTP and GRPC API
* Client connectors for popular frontend environments – for both web and mobile development
* Strict client protocol based on Protobuf schema
* Bidirectional transport support (WebSocket and SockJS) for full-featured communication
* Unidirectional transport support without need in client connectors - use native APIs (SSE, Fetch, WebSocket, GRPC)
* User authentication with a JWT or over connection request proxy to configured HTTP/GRPC endpoint
* Proper connection management and expiration control
* Various types of channels: anonymous, authenticated, private, user-limited
* Various types of subscriptions: client-side or server-side
* Transform RPC calls over WebSocket/SockJS to configured HTTP or GRPC endpoint call
* Presence information for channels (show all active clients in a channel)
* History information for channels (last messages published into a channel)
* Join/leave events for channels (client subscribed/unsubscribed)
* Automatic recovery of missed messages between reconnects over configured retention period
* Built-in administrative web panel
* Cross platform – works on Linux, macOS and Windows
* Ready to deploy (Docker, RPM/DEB packages, automatic TLS certificates, Prometheus instrumentation, Grafana dashboard)
* Open-source license

### Backing

This repository is hosted by [packagecloud.io](https://packagecloud.io/).

<a href="https://packagecloud.io/"><img height="46" width="158" alt="Private NPM registry and Maven, RPM, DEB, PyPi and RubyGem Repository · packagecloud" src="https://packagecloud.io/images/packagecloud-badge.png" /></a>
