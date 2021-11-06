No backwards incompatible changes here.

Improvements:

* Introducing a [granular proxy mode](https://centrifugal.dev/docs/server/proxy#granular-proxy-mode) for a fine-grained proxy configuration. Some background can be found in [#477](https://github.com/centrifugal/centrifugo/issues/477).

Also check out new tutorials in our blog (both examples can be run with single `docker compose up` command):

* [Centrifugo integration with NodeJS tutorial](https://centrifugal.dev/blog/2021/10/18/integrating-with-nodejs)
* [Centrifugo integration with Django – building a basic chat application](https://centrifugal.dev/blog/2021/11/04/integrating-with-django-building-chat-application)

Centrifugo [dashboard for Grafana](https://grafana.com/grafana/dashboards/13039) was updated and now uses [$__rate_interval](https://grafana.com/blog/2020/09/28/new-in-grafana-7.2-__rate_interval-for-prometheus-rate-queries-that-just-work/) function of Grafana.

This release is built with Go 1.17.3.
