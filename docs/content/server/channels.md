# Channels

Channel is a route for publication messages. Clients can subscribe to channel to receive events related to this channel – new publications, join/leave events etc. Also client must be subscribed on channel to get channel presence or history information.

Channel is just a string - `news`, `comments` are valid channel names.

Only ASCII symbols must be used in channel string.

**BUT!** You should know several things.

First, channel name length is limited by `255` characters by default (can be changed via configuration file option `channel_max_length`)

Second, `:`, `#` and `$` symbols have a special role in channel name.

### namespace channel boundary (:)

``:`` - is a channel namespace boundary.

If channel is `public:chat` - then Centrifugo will apply options to this channel from channel namespace with name `public`.

### user channel boundary (#)

`#` is a user boundary - separator to create private channels for users (user limited channels) without sending POST request to your web application. For example if channel is `news#42` then only user with ID `42` can subscribe on this channel (Centrifugo knows user ID as clients provide it in connection credentials).

Moreover you can provide several user IDs in channel name separated by comma: `dialog#42,43` – in this case only user with ID `42` and user with ID `43` will be able to subscribe on this channel.

This is useful for channels with static allowed users, for example for user personal messages channel, for dialog channel between certainly defined users. As soon as you need dynamic user access to channel this channel type does not suit well.

### $ private channel prefix ($)

If channel starts with `$` then it considered private. Subscription on private channel must be properly signed by your web application. Read special chapter in docs about private channel subscriptions.
