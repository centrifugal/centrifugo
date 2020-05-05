[![Join the chat at https://gitter.im/centrifugal/centrifugo](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/centrifugal/centrifugo?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge) [![Join the chat at https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ](https://img.shields.io/badge/Telegram-Group-blue.svg)](https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ)

Centrifugo is a real-time messaging server. It's language-agnostic and can be used in conjunction with application backend written in any programming language. Centrifugo runs as separate service and keeps persistent Websocket or SockJS connections from your application clients (from web browsers or other environments like iOS/Android apps). When you need to deliver event to your clients in real-time you publish it to Centrifugo API and Centrifugo then broadcasts event to all connected clients interested in this event (i.e. clients subscribed on event channel). In other words – this is a user-facing PUB/SUB server.

See server [documentation](https://centrifugal.github.io/centrifugo/).

![scheme](https://raw.githubusercontent.com/centrifugal/centrifugo/master/docs/content/images/scheme.png)

You can also find the following posts interesting:
* [Four years in Centrifuge](https://medium.com/@fzambia/four-years-in-centrifuge-ce7a94e8b1a8) – this is a story and motivation of Centrifugo
* [Building real-time messaging server in Go](https://medium.com/@fzambia/building-real-time-messaging-server-in-go-5661c0a45248) – this is a write-up about some Centrifugo internals and decisions

### How to install

Releases available as single executable files – just [download latest release](https://github.com/centrifugal/centrifugo/releases) for your platform, unpack and run.

If you are on MacOS:

```
brew tap centrifugal/centrifugo
brew install centrifugo
```

See official [Docker image](https://hub.docker.com/r/centrifugo/centrifugo/).

There are also [packages for 64-bit Debian, Centos and Ubuntu](https://packagecloud.io/FZambia/centrifugo).

### Demo

Try our [demo instance](https://centrifugo2.herokuapp.com/) on Heroku (admin password is `password`, secret is `secret`, API key is `api_key`). Or deploy your own Centrifugo instance in one click:

[![Deploy](https://www.herokucdn.com/deploy/button.svg)](https://heroku.com/deploy?template=https://github.com/centrifugal/centrifugo)

### Highlights

* Fast server capable to serve thousands of simultaneous connections
* Scale to millions of connections with Redis PUB/SUB, Redis Sentinel for high availability, consistent sharding support
* Easily integrates with any application – works as separate self-hosted service
* HTTP and GRPC API to communicate from your application backend (publish messages in channels etc)
* JSON and binary Protobuf Websocket client protocol
* SockJS polyfill for web browsers without Websocket support (JSON only)
* User authentication with JWT generated on your backend or over proxy request to configured HTTP endpoint
* Connection expiration control over JWT or request to configured HTTP endpoint
* JWT-based private channel authorization
* Transform RPC calls over WebSocket/SockJS to configured HTTP endpoint call 
* Presence information for channels (show all active clients in channel)
* History information for channels (last messages published into channel)
* Join/leave events for channels (client goes online/offline)
* Automatically recover missed messages between client reconnects over configured retention period
* Built-in admin web panel
* Cross platform – works on Linux, MacOS and Windows
* Ready to deploy (Docker, RPM/DEB packages, Nginx configuration, automatic Let's Encrypt TLS certificates, Prometheus/Graphite monitoring)
* MIT license
