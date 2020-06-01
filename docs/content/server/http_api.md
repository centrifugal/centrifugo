# Server HTTP API

HTTP API is a way to send commands to Centrifugo.

Why we need API?

If you look at configuration options you see an option called `publish` defined on configuration top-level and for a channel namespace. When turned on this option allows browser clients to publish into channels directly. If client publishes a message into channel directly – your application will not receive that message (it just goes through Centrifugo towards subscribed clients). This pattern can be useful sometimes but in most cases you first need to send new event from client to backend over non-Centrifugo transport (for example via AJAX request in web application), then process it on application backend side – probably validate, save into main app database – and then `publish` into Centrifugo using HTTP API so Centrifugo broadcast message to all clients subscribed on a channel.

Server API works on `/api` endpoint. It's very simple to use: you just have to send POST request with JSON command to this endpoint.

In this chapter we will look at API protocol internals - for new API client library authors and just if you are curious how existing API clients work.

API request is a POST HTTP request with `application/json` Content-Type and JSON payload in request body.

API protected by `api_key` set in Centrifugo configuration. I.e. `api_key` must be added to config, like:

```json
{
    ...
    "api_key": "<YOUR API KEY>"
}
```

This API key must be set in request `Authorization` header in this way:

```
Authorization: apikey <KEY>
```

Starting from Centrifugo v2.2.7 it's also possible to pass API key over URL query param. This solves some edge cases where it's not possible to use `Authorization` header. Simply add `?api_key=<YOUR API KEY>` query param to API endpoint. Keep in mind that passing API key in `Authorization` header is a recommended way. 

It's possible to disable API key check on Centrifugo side using `api_insecure` configuration option. Be sure to protect API endpoint by firewall rules in this case to prevent anyone in internet to send commands over your unprotected Centrifugo API. API key auth is not very safe for man-in-the-middle so recommended way is running Centrifugo with TLS (we are in 2018 in the end).

Command is a JSON object with two properties: `method` and `params`.

`method` is a name of command you want to call.
`params` is an object with command arguments.

There are several commands available. Let's investigate each of available server API commands.

### publish

Publish command allows publishing data into a channel. It looks like this:

```json
{
    "method": "publish",
    "params": {
        "channel": "chat", 
        "data": {
            "text": "hello"
        }
    } 
}
```

Let's apply all information said above and send publish command to Centrifugo. We will send request using `requests` library for Python. 

```python
import json
import requests

command = {
    "method": "publish",
    "params": {
        "channel": "docs", 
        "data": {
            "content": "1"
        }
    }
}

api_key = "YOUR_API_KEY"
data = json.dumps(command)
headers = {'Content-type': 'application/json', 'Authorization': 'apikey ' + api_key}
resp = requests.post("https://centrifuge.example.com/api", data=data, headers=headers)
print(resp.json())
```

The same using `httpie` console tool:

```bash
$ echo '{"method": "publish", "params": {"channel": "chat", "data": {"text": "hello"}}}' | http "localhost:8000/api" Authorization:"apikey KEY" -vvv
POST /api HTTP/1.1
Accept: application/json, */*
Accept-Encoding: gzip, deflate
Authorization: apikey KEY
Connection: keep-alive
Content-Length: 80
Content-Type: application/json
Host: localhost:8000
User-Agent: HTTPie/0.9.8

{
    "method": "publish",
    "params": {
        "channel": "chat",
        "data": {
            "text": "hello"
        }
    }
}

HTTP/1.1 200 OK
Content-Length: 3
Content-Type: application/json
Date: Thu, 17 May 2018 22:01:42 GMT

{}
```

In case of error response object will contain `error` field:

```bash
$ echo '{"method": "publish", "params": {"channel": "unknown:chat", "data": {"text": "hello"}}}' | http "localhost:8000/api" Authorization:"apikey KEY"
HTTP/1.1 200 OK
Content-Length: 55
Content-Type: application/json
Date: Thu, 17 May 2018 22:03:09 GMT

{
    "error": {
        "code": 102,
        "message": "namespace not found"
    }
}
```

`error` object contains error code and message - this also the same for other commands described below.

`publish` command is the main command you need. Again - remember that we have client API libraries that can help you avoid some boilerplate we just wrote and help to properly handle error responses from Centrifugo.

Let's look at other available commands:

### broadcast

Similar to `publish` but allows to send the same data into many channels.

```json
{
    "method": "broadcast",
    "params": {
        "channels": ["CHANNEL_1", "CHANNEL_2"],
        "data": {
            "text": "hello"
        }
    }
}
```

### unsubscribe

`unsubscribe` allows unsubscribing user from a channel. `params` is an object with two keys: `channel` and `user` (user ID you want to unsubscribe)

```json
{
    "method": "unsubscribe",
    "params": {
        "channel": "CHANNEL NAME",
        "user": "USER ID"
    }
}
```

### disconnect

`disconnect` allows disconnecting user by ID. `params` in an object with `user` key.

```json
{
    "method": "disconnect",
    "params": {
        "user": "USER ID"
    }
}
```

### presence

`presence` allows getting channel presence information (all clients currently subscribed on this channel). `params` is an object with `channel` key.

```json
{
    "method": "presence",
    "params": {
        "channel": "chat"
    }
}
```

Example:

```bash
fz@centrifugo: echo '{"method": "presence", "params": {"channel": "chat"}}' | http "localhost:8000/api" Authorization:"apikey KEY"
HTTP/1.1 200 OK
Content-Length: 127
Content-Type: application/json
Date: Thu, 17 May 2018 22:13:17 GMT

{
    "result": {
        "presence": {
            "c54313b2-0442-499a-a70c-051f8588020f": {
                "client": "c54313b2-0442-499a-a70c-051f8588020f",
                "user": "42"
            },
            "adad13b1-0442-499a-a70c-051f858802da": {
                "client": "adad13b1-0442-499a-a70c-051f858802da",
                "user": "42"
            }
        }
    }
}
```

### presence_stats

`presence_stats` allows getting short channel presence information.

```json
{
    "method": "presence_stats",
    "params": {
        "channel": "chat"
    }
}
```

Example:

```bash
$ echo '{"method": "presence_stats", "params": {"channel": "public:chat"}}' | http "localhost:8000/api" Authorization:"apikey KEY"
HTTP/1.1 200 OK
Content-Length: 43
Content-Type: application/json
Date: Thu, 17 May 2018 22:09:44 GMT

{
    "result": {
        "num_clients": 0,
        "num_users": 0
    }
}
```

### history

`history` allows getting channel history information (list of last messages published into channel).

`params` is an object with `channel` key:

```json
{
    "method": "history",
    "params": {
        "channel": "chat"
    }
}
```

Example:

```bash
$ echo '{"method": "history", "params": {"channel": "public:chat"}}' | http "localhost:8000/api" Authorization:"apikey KEY"
HTTP/1.1 200 OK
Content-Length: 87
Content-Type: application/json
Date: Thu, 17 May 2018 22:14:10 GMT

{
    "result": {
        "publications": [
            {
                "data": {
                    "text": "hello"
                },
                "uid": "BWcn14OTBrqUhTXyjNg0fg"
            }, {
                "data": {
                    "text": "hi!"
                },
                "uid": "Ascn14OTBrq14OXyjNg0hg"
            }
        ]
    }
}
```

### channels

`channels` allows getting list of active (with one or more subscribers) channels.

```json
{
    "method": "channels",
    "params": {}
}
```

Example:

```bash
$ echo '{"method": "channels", "params": {}}' | http "localhost:8000/api" Authorization:"apikey KEY"
HTTP/1.1 200 OK
Content-Length: 27
Content-Type: application/json
Date: Thu, 17 May 2018 22:08:31 GMT

{
    "result": {
        "channels": [
            "chat"
        ]
    }
}
```

Keep in mind that as `channels` API command returns all active channel snapshot it can be really heavy for massive deployments. At moment there is no way to paginate over channels list and we don't know a case where this could be useful and not error prone. At moment we mostly suppose that channels command will be used in development process and in not very massive Centrifugo setups (with no more than 10k channels). Also `channels` command is considered optional in engine implementations.

### info

`info` method allows getting information about running Centrifugo nodes.

```json
{
    "method": "info",
    "params": {}
}
```

Example:

```bash
$ echo '{"method": "info", "params": {}}' | http "localhost:8000/api" Authorization:"apikey KEY"
HTTP/1.1 200 OK
Content-Length: 184
Content-Type: application/json
Date: Thu, 17 May 2018 22:07:58 GMT

{
    "result": {
        "nodes": [
            {
                "name": "Alexanders-MacBook-Pro.local_8000",
                "num_channels": 0,
                "num_clients": 0,
                "num_users": 0,
                "uid": "f844a2ed-5edf-4815-b83c-271974003db9",
                "uptime": 0,
                "version": ""
            }
        ]
    }
}
```

## Command pipelining

It's possible to combine several commands into one request to Centrifugo. To do this use [JSON streaming](https://en.wikipedia.org/wiki/JSON_streaming) format. This can improve server throughput and reduce traffic travelling around.
