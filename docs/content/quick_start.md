# Quick start

Here we will build a very simple browser application with Centrifugo. It works in a way that users connect to Centrifugo over WebSocket, subscribe to channel and start receiving all messages published to that channel. In our case we will send a counter value to all channel subscribers to update it in all open browser tabs in real-time.

Hope you [installed Centrifugo](install.md). We can generate minimal required configuration file with the following command:

```
./centrifugo genconfig
```

It will generate `config.json` file in the same directory with content like this:

```
{
  "v3_use_offset": true,
  "token_hmac_secret_key": "46b38493-147e-4e3f-86e0-dc5ec54f5133",
  "admin_password": "ad0dff75-3131-4a02-8d64-9279b4f1c57b",
  "admin_secret": "583bc4b7-0fa5-4c4a-8566-16d3ce4ad401",
  "api_key": "aaaf202f-b5f8-4b34-bf88-f6c03a1ecda6"
}
```

Now we can start server, and let's start it with built-in admin web interface:

```
./centrifugo --config=config.json --admin
```

Now open http://localhost:8000 – and you should see Centrifugo admin web panel. Enter `admin_password` from configuration file to log in.

![Admin web panel](images/quick_start_admin.png)

Inside admin panel you should see that one Centrifugo node is running, and it does not have connected clients.

Now let's create `index.html` file with our simple app:

```html
<html>
    <head>
        <title>Centrifugo quick start</title>
    </head>
    <body>
        <div id="counter">-</div>
        <script src="https://cdn.jsdelivr.net/gh/centrifugal/centrifuge-js@2.6.2/dist/centrifuge.min.js"></script>
        <script type="text/javascript">
            const container = document.getElementById('counter')
            const centrifuge = new Centrifuge("ws://localhost:8000/connection/websocket");
            centrifuge.setToken("<TOKEN>");
            
            centrifuge.on('connect', function(ctx) {
                console.log("connected", ctx);
            });

            centrifuge.on('disconnect', function(ctx) {
                console.log("disconnected", ctx);
            });

            centrifuge.subscribe("channel", function(ctx) {
                container.innerHTML = ctx.data.value;
                document.title = ctx.data.value;
            });

            centrifuge.connect();
        </script>
    </body>
</html>
```

Note that we are using `centrifuge-js` 2.6.2 in this example, you better use its latest version for a moment of reading this.

We create an instance of client providing it Centrifugo default WebSocket endpoint address, then we subscribe to channel `channel` and provide callback function to process real-time messages. Then we call `connect` method to create WebSocket connection. 

You need to open this file in a browser, for example on MacOS:

```
open index.html
```

Or just enter sth like `file:///path/to/index.html` to browser address bar. In real application you will serve your HTML files with a proper web server – but for this simple example we don't need it.

Now if you look at browser developer tools or in Centrifugo logs you will notice that connection not successfully established:

```
2020-05-16 01:19:59 [INF] invalid connection token error="jwt: token format is not valid" client=45a1b8f4-d6dc-4679-9927-93e41c14ad93
2020-05-16 01:19:59 [INF] disconnect after handling command client=45a1b8f4-d6dc-4679-9927-93e41c14ad93 command="id:1 params:\"{\\\"token\\\":\\\"<TOKEN>\\\"}\" " reason="invalid token" user=
```

That's because client should provide valid JWT (JSON Web Token) to authenticate itself. This token **must be generated on your backend** and passed to client side. Since in our simple example we don't have application backend we can quickly generate example token for a user using `centrifugo` sub-command `gentoken`. Like this:

```
./centrifugo gentoken -u 123722
```

– where `-u` flag sets user ID. The output should be like:

```
HMAC SHA-256 JWT for user 123722 with expiration TTL 168h0m0s:
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM3MjIiLCJleHAiOjE1OTAxODYzMTZ9.YMJVJsQbK_p1fYFWkcoKBYr718AeavAk3MAYvxcMk0M
```

– you will have another token value since this one based on randomly generated `token_hmac_secret_key` from configuration file we created in the beginning of this tutorial.

Now we can copy generated HMAC SHA-256 JWT and paste it into `centrifuge.setToken` call instead of `<TOKEN>` placeholder in `index.html` file. I.e.:

```
centrifuge.setToken("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM3MjIiLCJleHAiOjE1OTAxODYzMTZ9.YMJVJsQbK_p1fYFWkcoKBYr718AeavAk3MAYvxcMk0M");
```

That's it! Now if you reload your browser tab – connection will be successfully established and client will subscribe to channel.

If you open developer tools and look at WebSocket frames panel you should see sth like this:

![Connected](images/quick_start_connected.png)

OK, the last thing we need to do here is publish new counter value to channel and make sure our app works properly.

We can do this over Centrifugo API sending HTTP request to default API endpoint `http://localhost:8000/api`, but let's do this over admin web panel first.

Open Centrifugo admin web panel in another browser tab and go to `Actions` section. Select publish action, insert channel name that you want to publish to – in our case this is string `channel` and insert into `data` area JSON like this:

```json
{
    "value": 1
}
```

![Admin publish](images/quick_start_publish.png)

Click `Submit` button and check out application browser tab – counter value must be immediately received and displayed.

Open several browser tabs with our app and make sure all tabs receive message as soon as you publish it.

![Message received](images/quick_start_message.png)

BTW, let's also look at how you can publish data to channel over Centrifugo API from a terminal using `curl` tool:

```bash
curl --header "Content-Type: application/json" \
  --header "Authorization: apikey aaaf202f-b5f8-4b34-bf88-f6c03a1ecda6" \
  --request POST \
  --data '{"method": "publish", "params": {"channel": "channel", "data": {"value": 2}}}' \
  http://localhost:8000/api
```

– where for `Authorization` header we set `api_key` value from Centrifugo config file generated above.

We did it! We built the simplest app with Centrifugo and its Javascript client. It does not have backend, it's not very useful to be honest, but it should give you an insight on how to start working with Centrifugo server. Read more about Centrifugo server in next documentations chapters – it can do much-much more than we just showed here. [Integration guide](guide.md) describes a process of idiomatic Centrifugo integration with your application backend.

### More examples

Several more examples located on Github – [check out this repo](https://github.com/centrifugal/examples)
