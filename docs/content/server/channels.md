# Channels

Channel is a route for publication messages. Clients can be subscribed to a channel to receive messages published to this channel – new publications, join/leave events (if enabled for a channel namespace) etc. Channel subscriber can also ask for channel presence or channel history information (if enabled for a channel namespace).

Channel is just a string - `news`, `comments` are valid channel names. Though this string has some predefined rules as we will see below.

Channel is an ephemeral entity – **you don't need to create it explicitly**. Channel created automatically by Centrifugo as soon as first client subscribes to it. As soon as last subscriber leaves channel - it's automatically cleaned up.

## Channel name rules

**Only ASCII symbols must be used in channel string**.

Channel name length limited by `255` characters by default (can be changed via configuration file option `channel_max_length`).

Several symbols in channel names reserved for Centrifugo internal needs:

* `:` – for namespace channel boundary (see below)
* `$` – for private channel prefix (see below)
* `#` – for user channel boundary (see below)
* `*` – for future Centrifugo needs
* `&` – for future Centrifugo needs
* `/` – for future Centrifugo needs

### namespace boundary (`:`)

``:`` – is a channel namespace boundary. Namespaces used to set custom options to a group of channels. Each channel belonging to the same namespace will have the same channel options. Read more about available channel options below.

If channel is `public:chat` - then Centrifugo will apply options to this channel from channel namespace with name `public`.

### private channel prefix (`$`)

If channel starts with `$` then it is considered `private`. Subscription on a private channel must be properly signed by your backend.

Use private channels if you pass sensitive data inside channel and want to control access permissions on your backend.

For example `$secrets` is a private channel, `$public:chat` - is a private channel that belongs namespace `public`.

Subscription request to private channels requires additional JWT from your backend. Read [detailed chapter about private channels](private_channels.md).

If you need a personal channel for a single user (or maybe channel for short and stable set of users) then consider using `user-limited` channel (see below) as a simpler alternative which does not require additional subscription token from your backend.

### user channel boundary (`#`)

`#` – is a user channel boundary. This is a separator to create personal channels for users (we call this `user-limited channels`) without need to provide subscription token.

For example if channel is `news#42` then only user with ID `42` can subscribe on this channel (Centrifugo knows user ID because clients provide it in connection credentials with connection JWT).

Moreover, you can provide several user IDs in channel name separated by a comma: `dialog#42,43` – in this case only user with ID `42` and user with ID `43` will be able to subscribe on this channel.

This is useful for channels with static list of allowed users, for example for single user personal messages channel, for dialog channel between certainly defined users. As soon as you need dynamic user access to channel this channel type does not suit well.

## Channel options

Let's look at configuration options related to channels. es published into that channel. The following options will affect channel behaviour.

### publish

`publish` (boolean, default `false`) – allow clients to publish messages into channels directly (from client side). Your application will never receive those messages. In idiomatic case all messages must be published to Centrifugo by your application backend using Centrifugo API. But this option can be useful when you want to build something without backend-side validation and saving into database. This option can also be useful for demos and prototyping real-time ideas. By default it's `false`.

### subscribe_to_publish

`subscribe_to_publish` (boolean, default `false`) - when `publish` option enabled client can publish into channel without being subscribed to it. This option enables automatic check that client subscribed on channel before allowing client to publish into channel.

### anonymous

`anonymous` (boolean, default `false`) – this option enables anonymous access (with empty `sub` claim in connection token). In most situations your application works with authenticated users so every user has its own unique id. But if you provide real-time features for public access you may need unauthorized access to some channels. Turn on this option and use empty string as user ID.

### presence

`presence` (boolean, default `false`) – enable/disable presence information. Presence is an information about clients currently subscribed on channel. By default this option is off so no presence information will be available for channels.

### presence_disable_for_client

`presence_disable_for_client` (boolean, default `false`, available since v2.2.3) – allows making presence calls available only for server side API. By default presence information is available for both client and server side APIs.

### join_leave

`join_leave` (boolean, default `false`) – enable/disable sending join(leave) messages when client subscribes on a channel (unsubscribes from channel).

### history_size

`history_size` (integer, default `0`) – history size (amount of messages) for channels. As Centrifugo keeps all history messages in memory it's very important to limit maximum amount of messages in channel history to reasonable value. `history_size` defines maximum amount of messages that Centrifugo will keep for **each** channel in namespace during history lifetime (see below). By default history size is `0` - this means that channels will have no history messages at all.

### history_lifetime

`history_lifetime` (integer, default `0`) – interval in seconds how long to keep channel history messages. As all history is storing in memory it is also very important to get rid of old history data for unused (inactive for a long time) channels. By default history lifetime is `0` – this means that channels will have no history messages at all. **So to turn on keeping history messages you should wisely configure both `history_size` and `history_lifetime` options**.

### history_recover

`history_recover` (boolean, default `false`) – when enabled Centrifugo will try to recover missed publications after a client reconnects for some reason (bad internet connection for example). Also when recovery feature is on Centrifugo tries to compensate at most once delivery of PUB/SUB messages checking client position inside stream, so you get at least once delivery guarantee in history retention period and history window size. By default, this feature is off. This option must be used in conjunction with reasonably configured message history for channel i.e. `history_size` and `history_lifetime` **must be set** (because Centrifugo uses channel history to recover messages). Also note that not all real-time events require this feature turned on so think wisely when you need this. When this option turned on your application should be designed in a way to tolerate duplicate messages coming from channel (currently Centrifugo returns recovered publications in order and without duplicates but this is implementation detail that can be theoretically changed in future). See more details about how recovery works in [special chapter](recover.md).

### history_disable_for_client

`history_disable_for_client` (boolean, default `false`, available since v2.2.3) – allows making history available only for a server side API. By default `false` – i.e. history calls are available for both client and server side APIs. History recovery mechanism if enabled will continue to work for clients anyway even if `history_disable_for_client` is on.

### server_side

`server_side` (boolean, default `false`, available since v2.4.0) – when enabled then all client-side subscription requests to channels in namespace will be rejected with `PermissionDenied` error.

### proxy_subscribe

`proxy_subscribe` (boolean, default `false`, available since v2.6.0) – turns on subscribe proxy, more info in [proxy chapter](proxy.md)

### proxy_publish

`proxy_publish` (boolean, default `false`, available since v2.6.0) – turns on publish proxy, more info in [proxy chapter](proxy.md)

## Channel options config example

Let's look how to set some of these options in a config:

```json
{
    "v3_use_offset": true,
    "token_hmac_secret_key": "my-secret-key",
    "api_key": "secret-api-key",
    "anonymous": true,
    "publish": true,
    "subscribe_to_publish": true,
    "presence": true,
    "join_leave": true,
    "history_size": 10,
    "history_lifetime": 300,
    "history_recover": true
}
```

## Channel namespaces

The last channel specific option is `namespaces`. `namespaces` are optional and if set must be an array of namespace objects. Namespace allows to configure custom options for channels starting with namespace name. This provides a great control over channel behaviour so you have a flexible way to define different channel options for different real-time features in your app.

Namespace has a name, and the same channel options (with same defaults) as described above.

* `name` - unique namespace name (name must consist of letters, numbers, underscores or hyphens and be more than 2 symbols length i.e. satisfy regexp `^[-a-zA-Z0-9_]{2,}$`).

If you want to use namespace options for a channel - you must include namespace name into channel name with `:` as separator:

`public:messages`

`gossips:messages`

Where `public` and `gossips` are namespace names from project `namespaces`.

All things together here is an example of `config.json` which includes some top-level channel options set and has 2 additional channel namespaces configured:

```json
{
    "v3_use_offset": true,
    "token_hmac_secret_key": "very-long-secret-key",
    "api_key": "secret-api-key",
    "anonymous": true,
    "publish": true,
    "presence": true,
    "join_leave": true,
    "history_size": 10,
    "history_lifetime": 30,
    "namespaces": [
        {
          "name": "public",
          "publish": true,
          "anonymous": true,
          "history_size": 10,
          "history_lifetime": 300,
          "history_recover": true
        },
        {
          "name": "gossips",
          "presence": true,
          "join_leave": true
        }
    ]
}
```

* Channel `news` will use globally defined channel options.
* Channel `public:news` will use `public` namespace's options.
* Channel `gossips:news` will use `gossips` namespace's options.

There is no inheritance in channel options and namespaces – so if for example you defined `presence: true` on top level of configuration and then defined namespace – that namespace won't have presence enabled - you must enable it for namespace explicitly. 
