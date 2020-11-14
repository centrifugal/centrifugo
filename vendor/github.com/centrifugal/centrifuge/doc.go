// Package centrifuge is a real-time messaging library that abstracts
// several bidirectional transports (Websocket, SockJS) and provides
// primitives to build scalable real-time applications with Go. It's
// also used as a core of Centrifugo server (https://github.com/centrifugal/centrifugo).
//
// Centrifuge library provides several features on top of plain Websocket
// implementation - read highlights in library README on Github –
// https://github.com/centrifugal/centrifuge.
//
// The API of this library is almost all goroutine-safe except cases where
// one-time operations like setting callback handlers performed, also your
// code inside event handlers should be synchronized since event handlers
// can be called concurrently. Library expects that code inside event handlers
// will not block. See more information about client connection lifetime and
// event handler order/concurrency in README on Github.
//
// Also check out examples in repo to see main library concepts in action.
package centrifuge
