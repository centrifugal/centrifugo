# Connection expiration

In authentication chapter we mentioned `exp` field in connection credentials that allows to expire client connection at some point of time. In this chapter we will look at details on what happens when Centrifugo detects that connection is going to expire.

So first you should do is enable client expiration mechanism in Centrifugo config:

```json
{
    ...
    "client_expire": true
}
```

Let's suppose that you set `exp` field to timestamp that will expire in 10 minutes and client connected to Centrifugo with this `exp`. During 10 mins connection will be kept by Centrifugo. When this time passed Centrifugo gives connection some time (configured, 25 seconds by default) to refresh its credentials and provide new actual `exp`.

When client first connects to Centrifugo it receives `ttl` value in connect reply. That `ttl` value contains number of seconds after which client must send `refresh` command with new credentials to Centrifugo. Centrifugo clients must handle this `ttl` field and automatically start refresh process.

For example in Javascript browser client AJAX POST request will be sent to your application when it's time to refresh credentials. By default this request goes to `/centrifuge/refresh` url endpoint. In response your server must return JSON with connection credentials. For example in python:

```python
    credentials = {
        'user': "42",
        'exp': "1521757509",
        'info': "",
        'sign': "...",
    }
    return json.dumps(credentials)
```

So you must just return the same connection credentials for `user` when rendering page initially. But with actual `exp`. Javascript client will then send them to Centrifugo server and connection will be refreshed for a time you set in `exp`.

If you don't want to refresh connection for this user - just return 403 Forbidden on refresh request to your application backend.

Javascript client also has options to hook into refresh mechanism to implement your custom way of refreshing. Other Centrifugo clients also should have hooks to refresh credentials but depending on client API for this can be different - see specific client docs.
