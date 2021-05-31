# HTTP API clients

Sending an API request to Centrifugo is a simple task to do in any programming language - this is just a POST request with JSON payload in body and `Authorization` header. See more in [special chapter](../server/http_api.md) in server section.

We have several official client libraries for different languages, so you don't have to construct proper HTTP requests manually:

* [cent](https://github.com/centrifugal/cent) for Python
* [phpcent](https://github.com/centrifugal/phpcent) for PHP
* [gocent](https://github.com/centrifugal/gocent) for Go

Also, there are API libraries created by community:

* [crystalcent](https://github.com/devops-israel/crystalcent) API client for Crystal language

Some integrations created by community:

* [laravel-centrifugo](https://github.com/denis660/laravel-centrifugo) integration with Laravel framework
* [laravel-centrifugo-broadcaster](https://github.com/opekunov/laravel-centrifugo-broadcaster) one more integration with Laravel framework to consider
* [CentrifugoBundle](https://github.com/fre5h/CentrifugoBundle) integration with Symfony framework
* [Django-instant](https://github.com/synw/django-instant) integration with Django framework

Also, keep in mind that Centrifugo [has GRPC API](../server/grpc_api.md) so you can automatically generate client API code for your language.
