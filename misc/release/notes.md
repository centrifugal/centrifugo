No backwards incompatible changes here.

Starting from this release we begin migration to new `offset` `uint64` client-server protocol field for Publication position inside history stream instead of currently used `seq` and `gen` (both `uint32`) fields. This `offset` field will be used in Centrifugo v3 by default. This change required to simplify working with history API, and due to this change history API can be later extended with pagination features.

Our client libraries `centrifuge-js`, `centrifuge-go` and `centrifuge-mobile` were updated to support `offset` field. If you are using these libraries then you can update `centrifuge-js` to at least `2.6.0`, `centrifuge-go` to at least `0.5.0` and `centrifuge-mobile` to at least `0.5.0` to work with the newest client-server protocol. As soon as you upgraded mentioned libraries you can enable `offset` support without waiting for Centrifugo v3 release with `v3_use_offset` option:

```json
{
  ...
  "v3_use_offset": true
}
```

All other client libraries except `centrifuge-js`, `centrifuge-go` and `centrifuge-mobile` do not support recovery at this moment and will only work with `offset` field in the future.

It's important to mention that `centrifuge-js`, `centrifuge-go` and `centrifuge-mobile` will continue to work with a server which is using `seq` and `gen` fields for recovery until Centrifugo v3 release. With Centrifugo v3 release those libraries will be updated to only work with `offset` field.

Command `centrifugo genconfig` will now generate config file with `v3_use_offset` option enabled. Documentation has been updated to suggest turning on this option for fresh installations.

Improvements:

* support [Redis Streams](https://redis.io/topics/streams-intro) - radically reduces amount of memory allocations during recovery in large history streams. This also opens a road to paginate over history stream in future releases, see description of new `redis_streams` option [in Redis engine docs](https://centrifugal.github.io/centrifugo/server/engines/#redis-streams)
* support [Redis Cluster](https://redis.io/topics/cluster-tutorial), client-side sharding between different Redis Clusters also works, see more [in docs](https://centrifugal.github.io/centrifugo/server/engines/#redis-cluster)
* faster HMAC-based JWT parsing
* faster Memory engine, possibility to expire history stream metadata (more [in docs](https://centrifugal.github.io/centrifugo/server/engines/#memory-engine))
* releases for Centos 8, Debian Buster, Ubuntu Focal Fossa
* new cli-command `centrifugo gentoken` to quickly generate HMAC SHA256 based connection JWT, [see docs](https://centrifugal.github.io/centrifugo/server/configuration/#gentoken-command)
* new cli-command `centrifugo checktoken` to quickly validate connection JWT while developing application, [see docs](https://centrifugal.github.io/centrifugo/server/configuration/#checktoken-command)

Fixes:

* fix server side subscriptions to private channels (were ignored before)
* fix `channels` counter update frequency in server `info` – this includes how fast `channels` counter updated in admin web interface (previously `num clients` and `num users` updated once in 3 seconds while `num channels` only once in a minute, now `num channels` updated once in 3 seconds too)

This release based on Go 1.14.x
