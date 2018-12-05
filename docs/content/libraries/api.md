# HTTP API clients

If you look at server API docs you will find that sending API request to Centrifugo is a very simple task to do in any programming language - this is just a POST request with JSON payload in body and `Authorization` header. See more in [special chapter](../server/api.md) in server section.

We have several client libraries for different languages so you don't have to construct proper HTTP requests manually:

* [cent](https://github.com/centrifugal/cent) for Python
* [phpcent](https://github.com/centrifugal/phpcent) for PHP
* [gocent](https://github.com/centrifugal/gocent) for Go
* [rubycent](https://github.com/centrifugal/rubycent) for Ruby (**not available for Centrifugo v2 yet**)
* [jscent](https://github.com/centrifugal/jscent) for NodeJS (**not available for Centrifugo v2 yet**)
