# Private channels

In channels chapter we mentioned private channels. This chapter has more information about private channel mechanism in Centrifugo.

All channels starting with `$` considered private. In this case your backend should additionally provide token for subscription request. The way how this token obtained varies depending on client implementation.

For example in Javascript client AJAX POST request automatically sent to `/centrifuge/subscribe` endpoint on every private channel subscription attempt. Other client libraries can provide a hook for your custom code that will obtain private channel subscription token from application backend.

Private channel subscription token is also JWT (like connection token described in [authentication chapter](authentication.md)). But it has different claims.

!!! note
    Connection token and private channel subscription token are different entities. Though both are JWT, and you can generate them using any JWT library.
    
!!! note
    Even when authorizing subscription to private channel with private subscription JWT you should set a proper connection JWT for a client as it provides user authentication details to Centrifugo.

!!! note
    When you need to use namespace for private channel then the name of namespace should be written after `$` symbol, i.e. if you have namespace name `chat` then private channel which belongs to that namespace must be written as sth like `$chat:stream`.

Supported JWT algorithms for private subscription tokens match algorithms to create connection JWT.

## Claims

Private channel subscription token claims are: `client`, `channel`, `info`, `b64info`, `exp` and `eto`. What do they mean? Let's describe in detail.

### client

Required. Client ID which wants to subscribe on a channel (**string**).

!!! note

    Centrifugo server sets a unique client ID for each incoming connection. This client ID regenerated on every reconnect. You must use this client ID for private channel subscription token. 
    If you are using [centrifuge-js](https://github.com/centrifugal/centrifuge-js) library then Client ID and Subscription Channels will be automaticaly added to POST request. In other cases refer to specific client documentation (in most cases you will have client ID in private subscription event context)

### channel

Required. Channel that client tries to subscribe to (**string**).

### info

Optional. Additional information for connection inside this channel (**valid JSON**).

### b64info

Optional. Additional information for connection inside this channel in base64 format (**string**).

### exp

Optional. This is standard JWT claim that allows to set private channel subscription token expiration time.

At moment if subscription token expires client connection will be closed and client will try to reconnect. In most cases you don't need this and should prefer using `exp` of connection token to deactivate connection. But if you need more granular per-channel control this may fit your needs.

Once `exp` set in token every subscription token must be periodically refreshed. Refer to specific client documentation in order to see how to refresh subscription tokens.

### eto

Optional. An `eto` boolean flag can be used to indicate that Centrifugo must only check token expiration but not turn on Subscription expiration checks on server side. This allows to implement one-time subcription tokens.

## Example

So to generate subscription token you can use something like this in Python (assuming client ID is `XXX` and private channel is `$gossips`):

```python
import jwt

token = jwt.encode({"client": "XXX", "channel": "$gossips"}, "secret", algorithm="HS256").decode()

print(token)
```

Where `"secret"` is the `token_hmac_secret_key` from Centrifugo configuration (we use HMAC tokens in this example which relies on shared secret key, for RSA tokens you need to use private key known only by your backend).
