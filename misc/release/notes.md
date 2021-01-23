No backwards incompatible changes here.

Improvements:

* [JSON Web Key](https://tools.ietf.org/html/rfc7517) support - see [pull request #410](https://github.com/centrifugal/centrifugo/pull/410) and [description in docs](https://centrifugal.github.io/centrifugo/server/authentication/#json-web-key-support)
* Support ECDSA algorithm for verifying JWT - see [pull request #420](https://github.com/centrifugal/centrifugo/pull/420) and updated [authentication docs chapter](https://centrifugal.github.io/centrifugo/server/authentication/)
* Various documentation clarifications - did you know that you can [use subscribe proxy instead of private channels](https://centrifugal.github.io/centrifugo/server/proxy/#subscribe-proxy) for example?

Fixes:

* Use more strict file permissions for a log file created (when using `log_file` option): `0666` -> `0644`

Other:

* Centrifugo repo [migrated from Travis CI to GH actions](https://github.com/centrifugal/centrifugo/issues/414), `golangci-lint` now works in CI
* Check out [a new community package](https://github.com/denis660/laravel-centrifugo) for Laravel that works with the latest version of framework
