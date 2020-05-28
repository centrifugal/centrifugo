# Channels

Channel is a route for publication messages. Clients can subscribe to channel to receive messages published to this channel – new publications, join/leave events (if enabled for a channel namespace) etc. Channel subscriber can ask for channel presence or channel history information (if enabled for a channel namespace) .

Channel is just a string - `news`, `comments` are valid channel names.

Channel is an ephemeral entity – **you don't need to create it explicitly**. Channel created automatically by Centrifugo as soon as first client subscribes to it. As soon as last subscriber leaves channel - it's automatically cleaned up.

### Channel name rules

Only ASCII symbols must be used in channel string.

Channel name length limited by `255` characters by default (can be changed via configuration file option `channel_max_length`).

Several symbols in channel names reserved for Centrifugo internal needs:

* `:` – for namespace channel boundary (see below)
* `$` – for private channel prefix (see below)
* `#` – for user channel boundary (see below)
* `*` – for future Centrifugo needs
* `&` – for future Centrifugo needs
* `/` – for future Centrifugo needs

### namespace channel boundary (:)

``:`` – is a channel namespace boundary.

If channel is `public:chat` - then Centrifugo will apply options to this channel from channel namespace with name `public`.

### private channel prefix ($)

If channel starts with `$` then it considered private. Subscription on a private channel must be properly signed by your web application.

For example `$secrets` is a private channel, `$public:chat` - is a private channel that belongs namespace `public`.

Read [special chapter in docs](private_channels.md) about private channel subscriptions.

### user channel boundary (#)

`#` – is a user channel boundary. This is a separator to create private channels for users (user limited channels) without sending POST request to your web application. For example if channel is `news#42` then only user with ID `42` can subscribe on this channel (Centrifugo knows user ID as clients provide it in connection credentials).

Moreover, you can provide several user IDs in channel name separated by a comma: `dialog#42,43` – in this case only user with ID `42` and user with ID `43` will be able to subscribe on this channel.

This is useful for channels with static allowed users, for example for user personal messages channel, for dialog channel between certainly defined users. As soon as you need dynamic user access to channel this channel type does not suit well.
