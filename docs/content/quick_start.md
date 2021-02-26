# Quick start

Here we will build a very simple browser application with Centrifugo. It works in a way that users connect to Centrifugo over WebSocket, subscribe to channel and start receiving all messages published to that channel. In our case we will send a counter value to all channel subscribers to update it in all open browser tabs in real-time.

First you need to [install Centrifugo](server/install.md). Below in this example we will use binary file release for simplicity. We can generate minimal required configuration file with the following command:

```
./centrifugo genconfig
```

It will generate `config.json` file in the same directory with content like this:

```json
{
  "v3_use_offset": true,
  "token_hmac_secret_key": "46b38493-147e-4e3f-86e0-dc5ec54f5133",
  "admin_password": "ad0dff75-3131-4a02-8d64-9279b4f1c57b",
  "admin_secret": "583bc4b7-0fa5-4c4a-8566-16d3ce4ad401",
  "api_key": "aaaf202f-b5f8-4b34-bf88-f6c03a1ecda6",
  "allowed_origins": []
}
```

Now we can start server, and let's start it with built-in admin web interface:

```console
./centrifugo --config=config.json --admin
```

We could also enable admin web interface by not using `--admin` flag but simply add `"admin": true` option to configuration file:

```json
{
  "v3_use_offset": true,
  "token_hmac_secret_key": "46b38493-147e-4e3f-86e0-dc5ec54f5133",
  "admin": true,
  "admin_password": "ad0dff75-3131-4a02-8d64-9279b4f1c57b",
  "admin_secret": "583bc4b7-0fa5-4c4a-8566-16d3ce4ad401",
  "api_key": "aaaf202f-b5f8-4b34-bf88-f6c03a1ecda6",
  "allowed_origins": []
}
```

And running:

```console
./centrifugo --config=config.json
```

Now open [http://localhost:8000](http://localhost:8000). You should see Centrifugo admin web panel. Enter `admin_password` from configuration file to log in.

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
        <script src="https://cdn.jsdelivr.net/gh/centrifugal/centrifuge-js@2.7.1/dist/centrifuge.min.js"></script>
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

Note that we are using `centrifuge-js` 2.7.1 in this example, you better use its latest version for a moment of reading this.

We create an instance of a client providing it Centrifugo default WebSocket endpoint address, then we subscribe to a channel `channel` and provide callback function to process real-time messages. Then we call `connect` method to create WebSocket connection. 

You need to serve this file with HTTP server, for example with Python 3 (in real Javascript application you will serve your HTML files with a proper web server – but for this simple example we can use a simple one):

```
python3 -m http.server 2000
```

If you don't have Python 3 then [this gist can be useful](https://gist.github.com/willurd/5720255).

Open [http://localhost:2000/](http://localhost:2000/).

Now if you look at browser developer tools or in Centrifugo logs you will notice a connection can not be successfully established:

```
2021-02-26 17:37:47 [INF] error checking request origin error="request Origin \"http://localhost:2000\" is not authorized for Host \"localhost:8000\""
```

That's because we are running our application on `localhost:2000` while Centrifugo runs on `localhost:8000`. We need to additionally configure `allowed_origins` option:

```json
{
  ...
  "allowed_origins": [
    "http://localhost:2000"
  ]
}
```

Restart Centrifugo after fixing a configuration file.

Now if you reload a browser window with an application you should see new information logs in server output:

```
2021-02-26 17:47:47 [INF] invalid connection token error="jwt: token format is not valid" client=45a1b8f4-d6dc-4679-9927-93e41c14ad93
2021-02-26 17:47:47 [INF] disconnect after handling command client=45a1b8f4-d6dc-4679-9927-93e41c14ad93 command="id:1 params:\"{\\\"token\\\":\\\"<TOKEN>\\\"}\" " reason="invalid token" user=
```

We still can not connect. That's because client should provide a valid JWT (JSON Web Token) to authenticate itself. This token **must be generated on your backend** and passed to a client side. Since in our simple example we don't have application backend we can quickly generate example token for a user using `centrifugo` sub-command `gentoken`. Like this:

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

OK, the last thing we need to do here is to publish new counter value to a channel and make sure our app works properly.

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

We did it! We built the simplest browser real-time app with Centrifugo and its Javascript client. It does not have backend, it's not very useful to be honest, but it should give you an insight on how to start working with Centrifugo server. Read more about Centrifugo server in next documentations chapters – it can do much-much more than we just showed here. [Integration guide](guide.md) describes a process of idiomatic Centrifugo integration with your application backend.

### More examples

Several more examples located on Github – [check out this repo](https://github.com/centrifugal/examples)
