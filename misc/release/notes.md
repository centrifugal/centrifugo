**Important** If you are using `rpm` or `deb` packages from packagecloud.io then you have to re-run the [installation method of your choice](https://packagecloud.io/FZambia/centrifugo/install) for Centrifugo repository. This is required to update GPG key used. This is a standard process that all packages hosted on packagecloud should do. 

Improvements:

* Redis TLS connection support - see [issue in Centrifuge lib](https://github.com/centrifugal/centrifuge/issues/23) and [updated docs](https://centrifugal.github.io/centrifugo/server/engines/#redis-engine)
* Do not send Authorization header in admin web interface when insecure admin mode enabled - helps to protect admin interface with basic authorization (see [#240](https://github.com/centrifugal/centrifugo/issues/240))

Fixes:

* Resubscribe only to shard subset of channels after reconnect to Redis ([issue](https://github.com/centrifugal/centrifuge/issues/25))
