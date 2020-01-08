This release contains several improvements to proxy feature introduced in v2.3.0, no backwards incompatible changes here.

Improvements:

* With `proxy_extra_http_headers` configuration option it's now possible to set a list of extra headers that should be copied from original client request to proxied HTTP request - see [#334](https://github.com/centrifugal/centrifugo/issues/334) for motivation and [updated proxy docs](https://centrifugal.github.io/centrifugo/server/proxy/)
* You can pass custom data in response to connect event and this data will be available in `connect` event callback context on client side. See [#332](https://github.com/centrifugal/centrifugo/issues/332) for more details
* Starting from this release `Origin` header is proxied to your backend by default - see [full list in docs](https://centrifugal.github.io/centrifugo/server/proxy/#proxy-headers)
