# Admin web interface

Centrifugo comes with builtin admin web interface.

It can:

* show current server general information and statistics from server nodes.
* call `publish`, `broadcast`, `unsubscribe`, `disconnect`, `history`, `presence`, `presence_stats`, `channels`, `info` server API commands. 

For `publish` command Ace JSON editor helps to write JSON to send into channel.

To enable admin interface you must run `centrifugo` with `--admin` and provide some security options in configuration file.

```bash
centrifugo --config=config.json --admin
```

Also you must set two options in config: `admin_password` and `admin_secret`:

```json
{
    ...,
    "admin_password": "<PASSWORD>",
    "admin_secret": "<SECRET>"
}
```

* `admin_password` – this is a password to log into admin web interface
* `admin_secret` - this is a secret key for authentication token set on successful login.

Make both strong and keep in secret.

After setting this in config go to http://localhost:8000 (by default) - and you should see web interface. Although there is `password` based authentication a good advice is to protect web interface by firewall rules in production.

If you don't want to use embedded web interface you can specify path to your own custom web interface directory:

```json
{
    ...,
    "admin_password": "<PASSWORD>",
    "admin_secret": "<SECRET>",
    "admin_web_path": "<PATH>"
}
```

This can be useful if you want to modify official [web interface code](https://github.com/centrifugal/web) in some way.

There is also an option to run Centrifugo in insecure admin mode - in this case you don't need to set `admin_password` and `admin_secret` in config – in web interface you will be logged in automatically without any password. Note that this is only an option for production if you protected admin web interface with firewall rules. Otherwise anyone in internet will have full access to admin functionality described above. To start Centrifugo with admin web interface in insecure admin mode run:

```
centrifugo --config=config.json --admin --admin_insecure
```
