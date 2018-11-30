# Private channels

In channels chapter we mentioned private channels. This chapter has more information about private channel mechanism in Centrifugo.

All channels starting with `$` considered private. In this case your backend should additionally provide token for subscription request. The way how this token is obtained varies depending on client implementation.

For example in Javascript client AJAX POST request is automatically sent to `/centrifuge/subscribe` endpoint on every private channel subscription attempt. Other client libraries can provide a hook for your custom code that will obtain private channel subscription token from application backend.

Private channel subscription token is also JWT (like connection token described in [authentication chapter](authentication.md)). But it has different claims.

!!! note
    Connection token and private channel subscription token are different entities. Though both are JWT and you can generate them using any JWT library.

## Claims

Private channel subscription token claims are: `client`, `channel`, `info`, `b64info` and `exp`. What do they mean? Let's describe in detail.

### client

Required. Client ID which wants to subscribe on channel (**string**).

### channel

Required. Channel that client tries to subscribe to (**string**).

### info

Optional. Additional information for connection regarding to channel (**valid JSON**).

### b64info

Optional. Additional information for connection regarding to channel in base64 format (**string**).

### exp

Optional. This is standard JWT claim that allows to set private channel subscription token expiration time.

At moment if subscription token expires client connection will be closed and client will try to reconnect. In most cases you don't need this and should prefer using `exp` of connection token to deactivate connection. But if you need more granular per-channel control this may fit your needs.

Once `exp` set in token every subscription token must be periodically refreshed. Refer to specific client documentation in order to see how to refresh subscription tokens.

## Example

So to generate subscription token you can use smth like this in Python (assuming client ID is `XXX` and private channel is `$gossips`):

```python
import jwt

token = jwt.encode({"client": "XXX", "channel": "$gossips"}, "secret", algorithm="HS256").decode()

print(token)
```

Again - the same `secret` from Centrifugo configuration is used to generate private channel JWT as was used to generate connection JWT. And as with connection JWT only `HS256` algorithm is supported at moment.
