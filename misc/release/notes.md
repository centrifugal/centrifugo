No backwards incompatible changes here.

Improvements:

* refreshed [documentation design](https://centrifugal.github.io/centrifugo/)
* new [Quick start](https://centrifugal.github.io/centrifugo/quick_start/) chapter for those who just start working with Centrifugo 
* faster marshal of disconnect messages into close frame texts, significantly reduces amount of memory allocations during server graceful shutdown in deployments with many connections
* one beautiful Centrifugo integration with Symfony framework from our community - [check it out](https://github.com/fre5h/CentrifugoBundle)

Fixes:

* add `Content-Type: application/json` header to outgoing HTTP proxy requests to app backend for better integration with some frameworks. [#368](https://github.com/centrifugal/centrifugo/issues/368)
* fix wrong channel name in Join messages sent to client in case of server-side subscription to many channels
* fix disconnect code unmarshalling after receiving response from HTTP proxy requests, it was ignored previously
