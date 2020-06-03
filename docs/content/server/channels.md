# Channels

Channel is a route for publication messages. Clients can subscribe to a channel to receive messages published to this channel – new publications, join/leave events (if enabled for a channel namespace) etc. Channel subscriber can also ask for channel presence or channel history information (if enabled for a channel namespace).

Channel is just a string - `news`, `comments` are valid channel names.

Channel is an ephemeral entity – **you don't need to create it explicitly**. Channel created automatically by Centrifugo as soon as first client subscribes to it. As soon as last subscriber leaves channel - it's automatically cleaned up.

### Channel name rules

**Only ASCII symbols must be used in channel string**.

Channel name length limited by `255` characters by default (can be changed via configuration file option `channel_max_length`).

Several symbols in channel names reserved for Centrifugo internal needs:

* `:` – for namespace channel boundary (see below)
* `$` – for private channel prefix (see below)
* `#` – for user channel boundary (see below)
* `*` – for future Centrifugo needs
* `&` – for future Centrifugo needs
* `/` – for future Centrifugo needs

### namespace channel boundary (:)

``:`` – is a channel namespace boundary. Namespaces used to set custom options to a group of channels. Each channel belonging to the same namespace will have the same channel options. Read more about available channel options in [configuration chapter](configuration.md).

If channel is `public:chat` - then Centrifugo will apply options to this channel from channel namespace with name `public`.

### private channel prefix ($)

If channel starts with `$` then it is considered `private`. Subscription on a private channel must be properly signed by your backend.

Use private channels if you pass sensitive data inside channel and want to control access permissions on your backend.

For example `$secrets` is a private channel, `$public:chat` - is a private channel that belongs namespace `public`.

Subscription request to private channels requires additional JWT from your backend. Read [detailed chapter about private channels](private_channels.md).

If you need a personal channel for a single user (or maybe channel for short and stable set of users) then consider using `user-limited` channel (see below) as a simpler alternative which does not require additional subscription token from your backend.

### user channel boundary (#)

`#` – is a user channel boundary. This is a separator to create personal channels for users (we call this `user-limited channels`) without need to provide subscription token.

For example if channel is `news#42` then only user with ID `42` can subscribe on this channel (Centrifugo knows user ID because clients provide it in connection credentials with connection JWT).

Moreover, you can provide several user IDs in channel name separated by a comma: `dialog#42,43` – in this case only user with ID `42` and user with ID `43` will be able to subscribe on this channel.

This is useful for channels with static list of allowed users, for example for single user personal messages channel, for dialog channel between certainly defined users. As soon as you need dynamic user access to channel this channel type does not suit well.
