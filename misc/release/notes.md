No backwards incompatible changes here.

Improvements:

* Possibility to modify data in publish proxy – see [#439](https://github.com/centrifugal/centrifugo/issues/439) and [updated docs for publish proxy](https://centrifugal.github.io/centrifugo/server/proxy/#publish-proxy)

Fixes:

* Use default timeouts for subscribe and publish proxy (1 second). Previously these proxy had no default timeout at all.

Centrifugo v2.8.5 based on Go 1.16.4
