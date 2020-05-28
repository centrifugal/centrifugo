# Authentication

When you are using [centrifuge](https://github.com/centrifugal/centrifuge) library from Go language you can implement any user authentication using middleware. In Centrifugo case you need to tell server who is connecting in well-known predefined way. This chapter will describe this mechanism.

When connecting to Centrifugo client must provide connection JWT token with several predefined credential claims. If you've never heard about JWT before - refer to [jwt.io](https://jwt.io/) page.

At moment **the only supported JWT algorithms are HMAC and RSA ** - i.e. HS256, HS384, HS512, RSA256, RSA384, RSA512. This can be extended later. RSA algorithm is available since v2.3.0 release.

We will use Javascript Centrifugo client here for example snippets for client side and [PyJWT](https://github.com/jpadilla/pyjwt) Python library to generate connection token on backend side.

To add HMAC secret key to Centrifugo add `token_hmac_secret_key` to configuration file:

```json
{
  "token_hmac_secret_key": "<YOUR-SECRET-STRING-HERE>",
  ...
}
```

To add RSA public key (must be PEM encoded string) add `token_rsa_public_key` option, ex:

```json
{
  "token_rsa_public_key": "-----BEGIN PUBLIC KEY-----\nMFwwDQYJKoZ...",
  ...
}
```

## Claims

Centrifugo uses the following claims in JWT: `sub`, `exp`, `info` and `b64info`. What do they mean? Let's describe in detail.

### sub

This is a standard JWT claim which must contain an ID of current application user (**as string**). 

If your user is not currently authenticated in your application but you want to let him connect to Centrifugo anyway you can use empty string as user ID in this `sub` claim. This is called anonymous access. In this case `anonymous` option must be enabled in Centrifugo configuration for channels that client will subscribe to.

### exp

This is an a UNIX timestamp seconds when token will expire. This is standard JWT claim - all JWT libraries for different languages provide an API to set it.

If `exp` claim not provided then Centrifugo won't expire any connections. When provided special algorithm will find connections with `exp` in the past and activate connection refresh mechanism. Refresh mechanism allows connection to survive and be prolonged. In case of refresh failure client connection will be eventually closed by Centrifugo and won't be accepted until new valid and actual credentials provided in connection token.

You can use connection expiration mechanism in cases when you don't want users of your app be subscribed on channels after being banned/deactivated in application. Or to protect your users from token leak (providing reasonably small time of expiration).

Choose `exp` value wisely, you don't need small values because refresh mechanism will hit your application often with refresh requests. But setting this value too large can lead to non very fast user connection deactivation. This is a trade off.

Read more about connection expiration in special chapter.

### info

This claim is optional - this is additional information about client connection that can be provided for Centrifugo. This information will be included in presence information, join/leave events and in channel publication message if it was published from client side.

### b64info

If you are using binary protobuf protocol you may want info to be custom bytes. Use this field in this case.

This field contains a `base64` representation of your bytes. After receiving Centrifugo will decode base64 back to bytes and will embed result into various places described above.

### channels

New in v2.4.0

Optional array of strings with server-side channels. See more details about [server-side subscriptions](server_subs.md).

## Examples

Let's look how to generate connection HS256 JWT in Python:

### Simplest token

```python
import jwt

token = jwt.encode({"sub": "42"}, "secret").decode()

print(token)
```

Note that we use the value of `secret` from Centrifugo config here (in this case `secret` value is just `secret`). The only two who must know secret key is your application backend which generates JWT and Centrifugo itself. You should never show secret key to your users. 

Then you can pass this token to your client side and use it when connecting to Centrifugo:

```javascript
var centrifuge = new Centrifuge("ws://localhost:8000/connection/websocket");
centrifuge.setToken(token);
centrifuge.connect();
```

### Token with expiration

Token that will be valid for 5 minutes:

```python
import jwt
import time

claims = {"sub": "42", "exp": int(time.time()) + 5*60}
token = jwt.encode(claims, "secret", algorithm="HS256").decode()
print(token)
```

### Token with additional connection info

```python
import jwt

claims = {"sub": "42", "info": {"name": "Alexander Emelin"}}
token = jwt.encode(claims, "secret", algorithm="HS256").decode()
print(token)
```

### Investigating problems with JWT

You can use [jwt.io](https://jwt.io/) site to investigate contents of your tokens. Also server logs usually contain some useful information.
