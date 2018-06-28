# Private channels

In channels chapter we mentioned private channels. This chapter has more information about private channel mechanism in Centrifugo. We will use Javascript client for client-side examples and Python for server-side code.

All channels starting with `$` considered private. In this case your backend should additionally provide token for subscription request. The way how this token is obtained varies depending on client implementation.

For example in Javascript client AJAX POST request is automatically made to `/centrifuge/subscribe` endpoint. Other client libraries can provide a hook for your custom code that will obtain private subscription token from application backend. 

The subscription token is similar to connection token. It's also JWT. But has different claims.

## Claims

Private subscription claims are: `client`, `channels`, `info` and `b64info`. What do they mean? Let's describe in detail.

### client

Required. Client ID which wants to subscribe on channel (**string**).

### channel

Required. Channel that client tries to subscribe to (**string**).

### info

Optional. Additional information for connection regarding to channel (**valid JSON**).

### b64info

Optional. Additional information for connection regarding to channel in base64 format (**string**).

## Example

So to generate subscription token you can use smth like this in Python (assuming client ID is `XXX` and private channel is `$gossips`):

```python
import jwt

token = jwt.encode({"client": "XXX", "channel": "gossips"}, "secret").decode()

print(token)
```

Again - the same `secret` from Centrifugo configuration is used to generate JWT.
