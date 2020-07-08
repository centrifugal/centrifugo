# SockJS

SockJS is a polyfill browser library which provides HTTP-based fallback transports in case when it's not possible to establish Websocket connection. This can happen in old client browsers or because of some proxy behind client and server that cuts of Websocket traffic. You can find more information on [SockJS project Github page](https://github.com/sockjs/sockjs-client).

If you have a requirement to work everywhere SockJS is the solution. SockJS will automatically choose best fallback transport if Websocket connection failed for some reason. Some of the fallback transports are:

* Eventsource (SSE)
* XHR-streaming
* Long-polling
* And more (see [SockJS docs](https://github.com/sockjs/sockjs-client))

One caveat when using SockJS is that **you need to use sticky sessions mechanism if you have many Centrifugo nodes running**. This mechanism usually supported by load balancers (for example Nginx). Sticky sessions mean that all requests from the same client will come to the same Centrifugo node. This is necessary because SockJS maintains connection session in process memory thus allowing bidirectional communication between a client and a server. Sticky mechanism not required if you only use one Centrifugo node on a backend. See how enable sticky sessions in Nginx in deploy section of this doc.

SockJS connection endpoint in Centrifugo is `/connection/sockjs`. SockJS does not support binary data, so you are limited in using JSON with it.
