No backwards incompatible changes here.

Fixes:

* Fix client reconnects due to `InsufficientState` errors. There were two scenarios when this could happen. The first one is using Redis engine with `seq`/`gen` legacy fields (i.e. not using **v3_use_offset** option). The second when publishing a lot of messages in parallel with Memory engine. Both scenarios should be fixed now.
* Fix non-working SockJS transport close with custom disconnect code: this is a regression introduced by v2.6.2
