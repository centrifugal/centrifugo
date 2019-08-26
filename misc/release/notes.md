No backwards incompatible changes here.

Improvements:

* Support for `tls-alpn-01` ACME challenge, see [#283](https://github.com/centrifugal/centrifugo/issues/283)

Fixes:

* fix running HTTP server several times when http-01 ACME challenge used, see [#288](https://github.com/centrifugal/centrifugo/issues/288)
