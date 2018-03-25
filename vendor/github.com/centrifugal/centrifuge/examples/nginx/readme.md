Nginx to proxy TLS traffic to Websocket and GRPC Centrifuge endpoints.

**Nginx >= 1.13.10 required**.

For example for MacOS [download](http://nginx.org/en/download.html) Nginx and unpack. Assuming you already have `openssl` installed via `brew`:

```
cd nginx-1.13.10/
./configure --prefix=/usr/local --with-http_ssl_module --with-http_v2_module --with-cc-opt="-I/usr/local/opt/openssl/include --with-ld-opt="-L/usr/local/opt/openssl/lib"
make && sudo make install
```

Passphrase for PEM here is `1234` but you can generate your own as shown below:

```
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -subj '/CN=localhost'
```

Start Nginx with `nginx.conf` from this repo:

```
sudo /usr/local/sbin/nginx -c `pwd`/nginx.conf
```

Then you can run `events` example in this repo and you connect to server using `wss://localhost:443/connection/websocket` endpoint (just change it in `index.html` file) and via GRPC using `localhost:443` as dial address (you can try naive `grpcclient` located nearby in examples).

Remember that as this example uses self-signed certificate you must skip verifying certificate check when connecting (or allow it in web browser in case of Websocket for example). With production certificates you don't need this of course.
