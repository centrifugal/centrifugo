[![Docs](https://img.shields.io/badge/docs-current-brightgreen.svg)](https://github.com/centrifugal/centrifugo)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/containous/traefik/blob/master/LICENSE.md)
[![Join the chat at https://gitter.im/centrifugal/centrifugo](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/centrifugal/centrifugo?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)

## Centrifugo

Centrifugo is a language-agnostic real-time server. It's main goal is to help adding real-time messages to your application. Language-agnostic means that it does not matter which programming language your application uses on frontend or backend sides - Centrifugo can work in conjunction with any. Real-time messages are messages that delivered to your clients almost immediately after some event happened - think live comments, real-time charts, updated counters.

There are several main transports Centrifugo supports at moment:

* SockJS (library that tries to establish Websocket connection and falls back to HTTP transports automatically in case of problems with Websockets)
* Websocket (JSON or binary Protobuf)
* GRPC

## Overview

Overview