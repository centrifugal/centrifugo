# Centrifugo PRO

Centrifugo becomes more popular and is used in many projects over the world. Since it's rather big project I need more and more time to maintain it healthy, improve client libraries and introduce new features.

I am considering to create a PRO version of Centrifugo. It may include some additional features on top of open-source Centrifugo version. This version will have some model of monetization, I am still evaluating whether it's reasonable to have and will be in demand. So please contact me (see email in [my GitHub profile](https://github.com/FZambia)) if you are interested in having additional features mentioned below or have any ideas on how to keep Centrifugo development sustainable.

## Real-time connection analytics with Clickhouse

This feature will allow exporting information about connections, subscriptions and client operations to [ClickHouse](https://clickhouse.tech/) thus providing real-time analytics with seconds delay. ClickHouse is fast and simple to operate with, and it allows effective data keeping for a window of time.  

## Client operation throttling

Throttle any operation per user ID using [token bucket algorithm](https://en.wikipedia.org/wiki/Token_bucket) and sharded Redis (or sharded Redis Cluster). Throttle connect, subscribe, publish, history, presence, RPC operations. You can also throttle RPC by method. 

## User online feature

Centrifugo has presence feature for channels. It works well, but sometimes you need a bit different functionality. What if all you want is get user online status?

User online feature will allow calling special RPC method from client side when your user makes some useful action in application (which means that users actively uses your app at moment), this will set user's `last online time` in Redis (again with sharding and Cluster support). On backend side you will have a bulk API to get online status of particular users. So you can get last time user was active in your app with some time approximation. Information will be kept in Redis for a configured time interval, then expire. This can be useful for chat applications. BTW it's possible to implement without Centrifugo but builtin possibility seems nice to have.  
