# TLS

TLS/SSL layer is very important not only for securing your connections but also to increase a
chance to establish Websocket connection. **In most situations you will put TLS termination task
on your reverse proxy/load balancing software such as Nginx**.

There are situations though when you want to serve secure connections by Centrifugo itself.

There are two ways to do this: using TLS certificate `cert` and `key` files that you've got
from your CA provider or using automatic certificate handling via [ACME](https://ietf-wg-acme.github.io/acme/) provider (only
[Let's Encrypt](https://letsencrypt.org/) at this moment).

### Using crt and key files

In first way you already have `cert` and `key` files. For development you can create self-signed
certificate - see [this instruction](https://devcenter.heroku.com/articles/ssl-certificate-self) as
example.

Then to start Centrifugo use the following command:

```
./centrifugo --config=config.json --tls --tls_key=server.key --tls_cert=server.crt
```

Or just use configuration file:

```json
{
  ...
  "tls": true,
  "tls_key": "server.key",
  "tls_cert": "server.crt"
}
```

And run:

```
./centrifugo --config=config.json
```

### Automatic certificates

For automatic certificates from Let's Encrypt add into configuration file:

```
{
  ...
  "tls_autocert": true,
  "tls_autocert_host_whitelist": "www.example.com",
  "tls_autocert_cache_dir": "/tmp/certs",
  "tls_autocert_email": "user@example.com",
  "tls_autocert_http": true,
  "tls_autocert_http_addr": ":80"
}
```

`tls_autocert` (boolean) says Centrifugo that you want automatic certificate handling using ACME provider.

`tls_autocert_host_whitelist` (string) is a string with your app domain address. This can be comma-separated
list. It's optional but recommended for extra security.

`tls_autocert_cache_dir` (string) is a path to a folder to cache issued certificate files. This is optional
but will increase performance.

`tls_autocert_email` (string) is optional - it's an email address ACME provider will send notifications
about problems with your certificates.

`tls_autocert_http` (boolean) is an option to handle http_01 ACME challenge on non-TLS port.

`tls_autocert_http_addr` (string) can be used to set address for handling http_01 ACME challenge (default is `:80`)

When configured correctly and your domain is valid (`localhost` will not work) - certificates
will be retrieved on first request to Centrifugo.

Also Let's Encrypt certificates will be automatically renewed.

There are tho options that allow Centrifugo to support TLS client connections from older
browsers such as Chrome 49 on Windows XP and IE8 on XP:

* `tls_autocert_force_rsa` - this is a boolean option, by default `false`. When enabled it forces
    autocert manager generate certificates with 2048-bit RSA keys.
* `tls_autocert_server_name` - string option, allows to set server name for client handshake hello.
    This can be useful to deal with old browsers without SNI support - see [comment](https://github.com/centrifugal/centrifugo/issues/144#issuecomment-279393819)

`grpc_api_tls_disable` boolean flag allows to disable TLS for GRPC API server but keep it on for HTTP endpoints.

### Custom TLS for GRPC API

Starting from Centrifugo v2.2.5 you can provide custom certificate files to configure TLS for GRPC API server in custom way.

* `grpc_api_tls` boolean flag enables TLS for GRPC API server, requires an X509 certificate and a key file
* `grpc_api_tls_cert` string provides a path to an X509 certificate file for GRPC API server
* `grpc_api_tls_key` string provides a path to an X509 certificate key for GRPC API server
