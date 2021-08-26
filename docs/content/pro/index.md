# Centrifugo PRO

!!!danger

    This is a documentation for Centrifugo v2. The latest Centrifugo version is v3. Go to the [centrifugal.dev](https://centrifugal.dev) for v3 docs.

Centrifugo becomes more popular and is used in many projects over the world. Since it's rather big project I need more and more time to maintain it healthy, improve client libraries and introduce new features.

I am considering to create a PRO version of Centrifugo. It may include some additional features on top of open-source Centrifugo version. This version will have some model of monetization, I am still evaluating whether it's reasonable to have and will be in demand.

An alternative to PRO version is our [Centrifugal OpenCollective](https://opencollective.com/centrifugal) organization – personally I prefer this way of becoming sustainable since it does not involve creating custom closed-source version of Centrifugo.

Please contact me (see email address in [my GitHub profile](https://github.com/FZambia)) if you are interested in having additional features mentioned below and have any ideas on how to keep Centrifugo development sustainable.

## Real-time connection analytics with Clickhouse

This feature will allow exporting information about connections, subscriptions and client operations to [ClickHouse](https://clickhouse.tech/) thus providing real-time analytics with seconds delay. ClickHouse is fast and simple to operate with, and it allows effective data keeping for a window of time.  

With analytics, one will be able to inspect the state of a system in detail with a connection resolution. This adds a greater observability, also the possibility to do data analysis and various reports.

## User operation throttling

Throttle any operation per user ID using [token bucket algorithm](https://en.wikipedia.org/wiki/Token_bucket) and sharded Redis (or sharded Redis Cluster). Throttle connect, subscribe, publish, history, presence, RPC operations. You can also throttle RPC by method. If user exceeded configured limit for an operation it will receive a special error in response.

## User active status

Centrifugo has presence feature for channels. It works well (for channels with reasonably small number of active subscribers though), but sometimes you need a bit different functionality. What if all you want is get specific user active status based on its recent activity in app? You can create a personal channel with presence enabled for each user. It will show that user connected to a server. But this won't show whether user did some actions in your app or just left it open while not actually using it.

User active status feature will allow calling a special RPC method from client side when your user makes some useful action in application (clicks on buttons, uses mouse – which means that user really uses your app at moment, not just connected to a server in background, and of course you still can call this RPC periodically), this RPC call will set user's `last active time` value in Redis (again with sharding and Cluster support). On backend side you will have access to a bulk API to get active status of particular users effectively. So you can get last time user was active in your app. Information about active status will be kept in Redis for a configured time interval, then expire.

This feature can be useful for chat applications when you need to get online/activity status for a list of buddies.

BTW it's already possible to implement without Centrifugo (we have this as advice in FAQ) but builtin possibility seems a nice thing to have.
