**Security warning**: take a closer look at new option `allowed_origins` **if you are using connect proxy feature**.

No backwards incompatible changes here.

Improvements:

* Possibility to set `allowed_origins` option ([#431](https://github.com/centrifugal/centrifugo/pull/431)). This option allows setting an array of allowed origin patterns (array of strings) for WebSocket and SockJS endpoints to prevent [Cross site request forgery](https://en.wikipedia.org/wiki/Cross-site_request_forgery) attack. This can be especially important when using [connect proxy](https://centrifugal.github.io/centrifugo/server/proxy/#connect-proxy) feature. If you are using JWT authentication then you should be safe. Note, that since you get an origin header as part of proxy request from Centrifugo it's possible to check allowed origins without upgrading to Centrifugo v2.8.3. See [docs](https://centrifugal.github.io/centrifugo/server/configuration/#allowed_origins) for more details about this new option
* Multi-arch Docker build support - at the moment for `linux/amd64` and `linux/arm64`. See [#433](https://github.com/centrifugal/centrifugo/pull/433)

Centrifugo v2.8.3 based on latest Go 1.16.2, Centrifugo does not vendor its dependencies anymore.
