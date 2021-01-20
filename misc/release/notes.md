No backwards incompatible changes here.

Improvements:

* [JSON Web Key](https://tools.ietf.org/html/rfc7517) support - see [pull request](https://github.com/centrifugal/centrifugo/pull/410) and [description in docs](https://centrifugal.github.io/centrifugo/server/authentication/#json-web-key-support)
* Various documentation clarifications - did you know that you can [use subscribe proxy instead of private channels](https://centrifugal.github.io/centrifugo/server/proxy/#subscribe-proxy) for example?

Fixes:

* Use more strict file permissions for a log file created (when using `log_file` option): `0666` -> `0644`

Other:

* Centrifugo repo migrated from Travis CI to GH actions
* Check out [a new community package](https://github.com/denis660/laravel-centrifugo) to integrate Laravel with Centrifugo
