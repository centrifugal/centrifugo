No backwards incompatible changes here.

Fixes:

* Fix proxy behavior for disconnected clients, should be now consistent between HTTP and GRPC proxy types.
* Fix `bufio: buffer full` error when unmarshalling large client protocol JSON messages.
* Fix `unexpected end of JSON input` errors in Javascript client with Centrifuge >= v0.18.0 when publishing formatted JSON (with new lines).

This release uses Go 1.17.1. We also added more tests for proxy package, thanks to [@silischev](https://github.com/silischev).
