// Package centrifuge is a real-time messaging library that abstracts
// several bidirectional transports (Websocket, SockJS) and provides
// primitives to build real-time applications with Go. It's also used as
// core of Centrifugo server.
//
// The API of this library is almost all goroutine-safe except cases where
// one-time operations like setting callback handlers performed.
//
// Centrifuge library provides several features on top of plain Websocket
// implementation - see full description in library README on Github – https://github.com/centrifugal/centrifuge.
//
// Also see examples in repo to see main library concepts in action.
package centrifuge
