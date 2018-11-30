# Centrifugo integration guide

This chapter aims to help you get started with Centrifugo. We will look at step-by-step workflow of integrating your application with Centrifugo providing links to relevant parts of this documentation.

As Centrifugo is language-agnostic and can be used together with any language/framework we won't be specific here about any backend or frontend technology your application can be built with. Only abstract steps which you can extrapolate to your stack.

So first of all let's look again at simplified scheme:

![Centrifugo scheme](images/scheme.png)

There are three parts involved into idiomatic Centrifugo usage scenario: your clients (frontend app), your application backend and Centrifugo. It's possible to use Centrifugo without any application backend involved but here we won't consider this use case. 

Here let's suppose you already have 2 of 3 elements: clients and backend. And you want to add Centrifugo for real-time events.

1) First you need to do is download/install Centrifugo server. See [install](server/install.md) chapter for details.

2) Create basic configuration file with `secret` and `api_key` set and then run Centrifugo. See [this chapter](server/configuration.md) for details about secret key and [chapter about API](server/api.md) for API description. The simplest way to do this automatically is by using `genconfig` command:

```
./centrifugo genconfig
```

– which will generate `config.json` file for you with all required fields.

3) In configuration file **of your application backend** register several variables: Centrifugo secret and Centrifugo API key you set on previous step and Centrifugo API address. By default API address is `http://localhost:8000/api`. You **must never reveal secret and API key to your users**.

4) Now your users can start connecting to Centrifugo. You should get client library (see [list of available client libraries](libraries/client.md)) for your application frontend. Every library has method to connect to Centrifugo. See information about Centrifugo connection endpoints [here](https://centrifugal.github.io/centrifugo/server/configuration/#advanced-endpoint-configuration). Every client should provide connection token (JWT) on connect. You must generate this token on your backend side using Centrifugo secret key you set to backend configuration. See how to generate this JWT [in special chapter](server/authentication.md). You pass this token from backend to your frontend app (pass it in template context or use separate request from client side to get user specific JWT from backend side). And use this token when connecting to Centrifugo (for example browser client has special method `setToken`).

5) After connecting to Centrifugo subscribe clients to channels they are interested in. See more about channels in [special chapter](server/channels.md). All client libraries provide a way to handle messages coming to client from channel after subscribing to it.

6) So everything should work now – as soon as user opens some page of your application it must successfully connect to Centrifugo and subscribe to channel (or channels). Now let's imagine you want to send real-time message to users subscribed on specific channel. This message can be a reaction on some event happened in your app: someone posted new comment, administrator just created new post, user pressed like button etc. Anyway this is an event your backend just got and you want to immediately share it with interested users. You can do this using Centrifugo [HTTP API](server/api.md). To simplify your life [we have several API libraries](libraries/api.md) for different languages. You can publish message into channel using one of those libraries or you can simply [follow API description](server/api.md) to construct API request yourself - this is very simple. As soon as you published message to channel it must be delivered to your client.

7) To put this all into production you need to deploy Centrifugo on your production server. To help you with this we have many things like Docker image, `rpm` and `deb` packages, Nginx configuration. You can find more information in Deploy section of this doc.

8) Don't forget to [monitor](server/monitoring.md) your production Centrifugo setup.

That's all for basics. Documentation actually covers lots of other concepts Centrifugo server has: like scalability, private channels, admin web interface, SockJS fallback, Protobuf support and more. And don't forget to read our [FAQ](faq.md).
