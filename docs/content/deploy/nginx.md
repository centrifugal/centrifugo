# Nginx configuration

Although it's possible to  use Centrifugo without any reverse proxy before it,
it's still a good idea to keep Centrifugo behind mature reverse proxy to deal with
edge cases when handling HTTP/Websocket connections from the wild. Also you probably
want some sort of load balancing eventually between Centrifugo nodes so that proxy
can be such a balancer too.

In this section we will look at [Nginx](http://nginx.org/) configuration to deploy Centrifugo.

Minimal Nginx version – **1.3.13** because it was the first version that can proxy
Websocket connections.

There are 2 ways: running Centrifugo server as separate service on its own
domain or embed it to a location of your web site (for example to `/centrifugo`).

## Separate domain for Centrifugo

```
upstream centrifugo {
    # Enumerate all upstream servers here
    #sticky;
    ip_hash;
    server 127.0.0.1:8000;
    #server 127.0.0.1:8001;
}

map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

#server {
#	listen 80;
#	server_name centrifugo.example.com;
#	rewrite ^(.*) https://$server_name$1 permanent;
#}

server {

    server_name centrifugo.example.com;

    listen 80;

    #listen 443;
    #ssl on;
    #ssl_protocols TLSv1 TLSv1.1 TLSv1.2;
    #ssl_ciphers AES128-SHA:AES256-SHA:RC4-SHA:DES-CBC3-SHA:RC4-MD5;
    #ssl_certificate /etc/nginx/ssl/wildcard.example.com.crt;
    #ssl_certificate_key /etc/nginx/ssl/wildcard.example.com.key;
    #ssl_session_cache shared:SSL:10m;ssl_session_timeout 10m;

    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    gzip on;
    gzip_min_length 1000;
    gzip_proxied any;

    # Only retry if there was a communication error, not a timeout
    # on the Tornado server (to avoid propagating "queries of death"
    # to all frontends)
    proxy_next_upstream error;

    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Scheme $scheme;
    proxy_set_header Host $http_host;

    location /connection {
        proxy_pass http://centrifugo;
        proxy_buffering off;
        keepalive_timeout 65;
        proxy_read_timeout 60s;
        proxy_http_version 1.1;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Scheme $scheme;
        proxy_set_header Host $http_host;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
    
    location / {
        proxy_pass http://centrifugo;
    }

    error_page   500 502 503 504  /50x.html;

    location = /50x.html {
        root   /usr/share/nginx/html;
    }
}
```

## Embed to a location of web site

```
upstream centrifugo {
    # Enumerate all the Tornado servers here
    #sticky;
    ip_hash;
    server 127.0.0.1:8000;
    #server 127.0.0.1:8001;
}

map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {

    # ... your web site Nginx config

    location /centrifugo/ {
        rewrite ^/centrifugo/(.*)        /$1 break;
        proxy_pass_header Server;
        proxy_set_header Host $http_host;
        proxy_redirect off;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Scheme $scheme;
        proxy_pass http://centrifugo;
    }

    location /centrifugo/connection {
        rewrite ^/centrifugo(.*)        $1 break;

        proxy_next_upstream error;
        gzip on;
        gzip_min_length 1000;
        gzip_proxied any;
        proxy_buffering off;
        keepalive_timeout 65;
        proxy_pass http://centrifugo;
        proxy_read_timeout 60s;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Scheme $scheme;
        proxy_set_header Host $http_host;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
}
```

## sticky

You may be noticed commented `sticky;` directive in nginx upstream configuration.

When using SockJS and client connects to Centrifugo - SockJS session created - and
to communicate client must send all next requests to the same upstream backend.

In this configuration we use `ip_hash;` directive to proxy clients with the same ip
address to the same upstream backend.

But `ip_hash;` is not the best choice in this case, because there could be situations
where a lot of different browsers are coming with the same IP address (behind proxies)
and the load balancing system won't be fair. Also fair load balancing does not work
during development - when all clients connecting from localhost.

So the best solution would be using something like [nginx-sticky-module](https://bitbucket.org/nginx-goodies/nginx-sticky-module-ng/overview)
which uses setting a special cookie to track the upstream server for client.

## worker_connections

You may also need to update `worker_connections` option of Nginx:

```
events {
    worker_connections 40000;
}
```

## Upstream keepalive

See [chapter about operating system tuning](tuning.md) for more details.
