This section describes client-server transports that Centrifugo supports and its specifics. Client-server transports used to connect to Centrifugo from a frontend of your application.

At moment Centrifugo supports the following client-server transports:

* **Websocket** – with JSON or binary protobuf protocol.
* **SockJS** – only supports JSON protocol

Having both of these transport means that it's possible to connect to Centrifugo from everywhere.

Since Centrifugo has its own protocol for client-server communication (for authentication, subscriptions, publishing messages to channels, calling RPC etc) transports above wrapped by our [client libraries](../libraries/client.md).
