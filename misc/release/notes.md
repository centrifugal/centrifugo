This release contains changes in metric paths exported to Graphite, you may need to fix your dashboard when upgrading. Otherwise everything in backwards compatible.

Improvements:

* Refactored export to Graphite, you can now control aggregation interval using `graphite_interval` option (in seconds), during refactoring some magical path transformation was removed so now we have more predictable path generation. Though Graphite paths changed with this refactoring
* Web interface rewritten using modern Javascript stack - latest React, Webpack instead of Gulp, ES6 syntax 
* Aggregated metrics also added to `info` command reply. This makes it possible to look at metrics in admin panel too when calling `info` command
* More options can be set over environment variables – see [#254](https://github.com/centrifugal/centrifugo/issues/254)
* Healthcheck endpoint – see [#252](https://github.com/centrifugal/centrifugo/issues/252)
* New important chapter in docs – [integration guide](https://centrifugal.github.io/centrifugo/guide/)
* Support setting `api_key` when using `Deploy on Heroku` button
* Better timeout handling in Redis engine – client timeout is now bigger than default `redis_read_timeout` so application can more reliably handle errors

Fixes:

* Dockerfile had no correct `WORKDIR` set so it was only possible to use absolute config file path, now this is fixed in [this commit](https://github.com/centrifugal/centrifugo/commit/08be85223aa849d9996c16971f9d049125ade50c) 
* Show node version in admin web panel
* Fix possible goroutine leak on client connection close, [commit](https://github.com/centrifugal/centrifuge/commit/a70909c2a2677932fcef0910525ea9497ff9acf2)
