# Authentication

When you are using [centrifuge](https://github.com/centrifugal/centrifuge) library from Go language you can implement any user authentication. In Centrifugo case you need to tell server who is connecting in well-known predefined way. This chapter will describe this mechanism. We will use Javascript client here for example snippets.

When connecting to Centrifugo client must provide connection authentication credentials. Credentials are several string fields: `user`, `exp`, `info` and `sign`. What do they mean? Let's describe in detail.

### user

This is simple - it's an ID of current application user (**as string**). 

If your user not currently authenticated in your application but you want to let him connect to Centrifugo anyway you can use empty string as user ID. This is called anonymous access. In this case `anonymous` option must be enabled in Centrifugo configuration for channels that client will subscribe to.

### exp

This is a UNIX timestamp seconds when user connection must be considered expired. By default Centrifugo won't expire any connections but you can enable expiration mechanism in its options. When on this mechanism will find connections with `exp` in the past and activate connection refresh mechanism. Refresh mechanism allows connection to survive and be prolonged. In case of refresh failure client connection will be eventually closed by Centrifugo and won't be accepted until new valid and actual credentials provided.

You can use connection expiration mechanism in cases when you don't want users of your app be subscribed on channels after being banned/deactivated in application.

Choose `exp` value wisely, you don't need to small values because refresh mechanism will hit your application often. But setting this value too large can lead to non very fast user connection  deactivation. This is trade off.

Let's look how we can create `exp` value in Python and make client credentials valid for 10 minutes:

```python
import time

exp = str(int(time.time()) + 10 * 60)
```

Read more about connection expiration in special chapter.

### info

This field is optional - this is additional information about client connection that can be provided fot Centrifugo. This information will be included in presence information, join/leave events and in channel publication message if it was published from client side.

This field contains `base64` string. In case of using JSON protocol this is JSON object converted to UTF-8 bytes and then encoded with `base64` codec. In case of using Protobuf protocol this can be random bytes also encoded to `base64` format. Base64 encoding was chosen to help pass bytes in web environment. 

After receiving Centrifugo will decode base64 back to bytes and will embed them into various places described above.

### sign

Finally `sign`. It could be very simple for malicious client to provide wrong userID, exp or info without this field. `sign` is an SHA256 HMAC hexdigest that client must provide to prove that connection credentials are valid and not modified.

You must generate this HMAC sign on your backend based on secret key you set in Centrifugo config. The only two who must know that secret key is your application backend and Centrifugo itself. You should never show secret key to your users. Our API libraries have helper functions to help your backend generate this sign. But this is not very hard even without our libraries:

For example in Python 3:


```python
def generate_client_sign(secret, user, exp, info=""):
    sign = hmac.new(bytes(secret, "utf-8"), digestmod=sha256)
    sign.update(bytes(user, "utf-8"))
    sign.update(bytes(exp, "utf-8"))
    sign.update(bytes(info, "utf-8"))
    return sign.hexdigest()


secret = "secret"
user = "42"
exp = str(int(time.time()) + 10 * 60)

sign = generate_client_sign(secret, user, exp)
``` 

Then you can pass credentials to your client side to use when connection to Centrifugo:

```javascript
var centrifuge = new Centrifuge("ws://localhost:8000/connection/websocket");

centrifuge.setCredentials({
    "user": user,
    "exp": exp,
    "info": "",
    "sign": sign
});

centrifuge.connect();
```
