# Centrifugo

_Scalable real-time messaging server in a language-agnostic way._

[![CI](https://github.com/centrifugal/centrifugo/actions/workflows/test.yml/badge.svg)](https://github.com/centrifugal/centrifugo/actions/workflows/test.yml)
[![Release](https://img.shields.io/github/v/release/centrifugal/centrifugo?sort=semver)](https://github.com/centrifugal/centrifugo/releases)
[![Docker Pulls](https://img.shields.io/docker/pulls/centrifugo/centrifugo)](https://hub.docker.com/r/centrifugo/centrifugo)
[![License](https://img.shields.io/github/license/centrifugal/centrifugo)](https://github.com/centrifugal/centrifugo/blob/master/LICENSE)
[![Telegram](https://img.shields.io/badge/Telegram-join-26A5E4?logo=telegram&logoColor=white)](https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ)
[![Discord](https://img.shields.io/badge/Discord-join-5865F2?logo=discord&logoColor=white)](https://discord.gg/tYgADKx)

Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (aka EventSource), GRPC, WebTransport). Centrifugo has the concept of channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

The core idea of Centrifugo is simple – it's a PUB/SUB server on top of modern real-time transports:

<img src="https://centrifugal.dev/img/protocol_pub_sub.png?v=2" alt="Centrifugo PUB/SUB diagram" />

Your backend communicates with Centrifugo over a simple HTTP or GRPC API to publish messages into channels, while clients subscribe to those channels using one of the official SDKs (or the unidirectional approach with no SDK dependency).

## Quick start

Spin up Centrifugo with its embedded admin UI using Docker:

```bash
docker run -it --rm -p 8000:8000 centrifugo/centrifugo:latest centrifugo \
  --client.insecure --admin.enabled --admin.insecure
# ^ insecure flags: for local trial only, never use in production
```

Then open http://localhost:8000 to reach the admin UI, where you can watch live connections and publish messages into channels. The `insecure` flags above remove authentication for a frictionless local trial only – never use them in production.

For native binaries, Homebrew, packages, and production configuration see the [installation instructions](https://centrifugal.dev/docs/getting-started/installation), and follow the [quickstart tutorial](https://centrifugal.dev/docs/getting-started/quickstart) to connect your first client.

## Why Centrifugo

The hard part is to make the PUB/SUB concept production-ready, efficient, flexible and available from different application environments. Centrifugo is a mature solution that already helped many projects with adding real-time features and scaling towards many concurrent connections. It provides a set of features not available in other open-source solutions in the area:

* Efficient real-time transports: WebSocket, HTTP-streaming, Server-Sent Events, GRPC, WebTransport (experimental)
* Built-in scalability with Redis (or Redis Cluster, or Redis-compatible storage – ex. AWS Elasticache, Valkey, KeyDB, DragonflyDB, etc), PostgreSQL or Nats.
* Simple HTTP and GRPC server API to communicate with Centrifugo from the app backend
* Asynchronous PostgreSQL and Kafka consumers to support transactional outbox and CDC patterns
* Flexible connection authentication mechanisms: JWT and proxy-like (via request from Centrifugo to the backend)
* Channel subscription multiplexing over a single connection
* Rich subscription model: stream (client-side and server-side), map, shared poll, and proxy subscription streams
* Various channel permission strategies, channel namespace concept
* Hot message history in channels, with automatic message recovery upon reconnect, cache recovery mode (deliver latest publication immediately upon subscription)
* Delta compression in channels based on Fossil algorithm
* Online channel presence information, with join/leave notifications
* A way to send RPC calls to the backend over the real-time connection
* Strict and effective client protocol wrapped by several official SDKs
* JSON and binary Protobuf message transfer, with optimized serialization and built-in batching
* Beautiful embedded admin web UI
* Great observability with lots of Prometheus metrics exposed and official Grafana dashboard
* And much more, visit [Centrifugo documentation site](https://centrifugal.dev)

## Client SDKs

Official SDKs wrap Centrifugo's bidirectional protocol and handle reconnects, subscription state, message recovery, and more across platforms:

| Platform / Language             | SDK                                                                   |
|---------------------------------|-----------------------------------------------------------------------|
| JavaScript (browser, Node.js, React Native) | [centrifuge-js](https://github.com/centrifugal/centrifuge-js) |
| Dart / Flutter (mobile and web) | [centrifuge-dart](https://github.com/centrifugal/centrifuge-dart)     |
| Swift / native iOS              | [centrifuge-swift](https://github.com/centrifugal/centrifuge-swift)   |
| Java (Android, JVM)             | [centrifuge-java](https://github.com/centrifugal/centrifuge-java)     |
| Python (asyncio)                | [centrifuge-python](https://github.com/centrifugal/centrifuge-python) |
| Go                              | [centrifuge-go](https://github.com/centrifugal/centrifuge-go)         |
| .NET / MAUI / Unity [WIP]       | [centrifuge-csharp](https://github.com/centrifugal/centrifuge-csharp) |

For simple use cases Centrifugo also supports a unidirectional approach (WebSocket, SSE, HTTP-streaming, GRPC) that needs no SDK – any standard client for the transport works. See the [transports overview](https://centrifugal.dev/docs/transports/overview) and the [client protocol spec](https://centrifugal.dev/docs/transports/client_api) for details.

## Documentation

* [Centrifugo official documentation site](https://centrifugal.dev)
* [Installation instructions](https://centrifugal.dev/docs/getting-started/installation)
* [Getting started tutorial](https://centrifugal.dev/docs/getting-started/quickstart)
* [Design overview and idiomatic usage](https://centrifugal.dev/docs/getting-started/design)
* [Build a WebSocket chat/messenger app with Centrifugo](https://centrifugal.dev/docs/tutorial/intro) tutorial
* [Centrifugal blog](https://centrifugal.dev/blog)
* [FAQ](https://centrifugal.dev/docs/faq)
* [Examples repository](https://github.com/centrifugal/examples)

## Community

* [Telegram](https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ)
* [Discord](https://discord.gg/tYgADKx)
* [X (Twitter)](https://twitter.com/centrifugalabs)

Found a bug or have an idea? Open an [issue](https://github.com/centrifugal/centrifugo/issues/new/choose).
Want to contribute? See [CONTRIBUTING.md](CONTRIBUTING.md).

## Backing

This repository is hosted by [packagecloud.io](https://packagecloud.io/).

<a href="https://packagecloud.io/"><img height="46" width="158" alt="Private NPM registry and Maven, RPM, DEB, PyPi and RubyGem Repository · packagecloud" src="https://packagecloud.io/images/packagecloud-badge.png" /></a>

Also thanks to [JetBrains](https://www.jetbrains.com/) for supporting OSS (most of the code here written in Goland):

<a href="https://www.jetbrains.com/"><img height="140" src="https://resources.jetbrains.com/storage/products/company/brand/logos/jb_beam.png" alt="JetBrains logo"></a>
