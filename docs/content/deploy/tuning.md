# Tuning infrastructure to handle persistent connections

!!!danger

    This is a documentation for Centrifugo v2. The latest Centrifugo version is v3. Go to the [centrifugal.dev](https://centrifugal.dev) for v3 docs.

As Centrifugo deals with lots of persistent connections your operating system and server infrastructure must be ready for it.

### Open files limit

You should increase a max number of open files Centrifugo process can open if you want to handle more connections.

To get the current open files limit run:

```
ulimit -n
```

On Linux you can check limits for a running process using:

```
cat /proc/<PID>/limits
```

The open file limit shows approximately how many clients your server can handle. Each connection consumes one file descriptor. On most operating systems this limit is 128-256 by default.

See [this document](https://docs.riak.com/riak/kv/2.2.3/using/performance/open-files-limit.1.html) to get more info on how to increase this number.

If you install Centrifugo using RPM from repo then it automatically sets max open files limit to 65536.

You may also need to increase max open files for Nginx (or any other proxy before Centrifugo).

### Ephemeral port exhaustion

Ephemeral ports exhaustion problem can happen between your load balancer and Centrifugo server. If your clients connect directly to Centrifugo without any load balancer or reverse proxy software between then you are most likely won't have this problem. But load balancing is a very common thing.

The problem arises due to the fact that each TCP connection uniquely identified in the OS by the 4-part-tuple:

```
source ip | source port | destination ip | destination port
```

On load balancer/server boundary you are limited in 65536 possible variants by default. Actually due to some OS limits not all 65536 ports are available for usage (usually about 15k ports available by default).

In order to eliminate a problem you can:

* Increase the ephemeral port range by tuning `ip_local_port_range` option
* Deploy more Centrifugo server instances to load balance across
* Deploy more load balancer instances
* Use virtual network interfaces

See [a post in Pusher blog](https://making.pusher.com/ephemeral-port-exhaustion-and-how-to-avoid-it/) about this problem and more detailed solution steps.

### Sockets in TIME_WAIT state

On load balancer/server boundary one more problem can arise: sockets in TIME_WAIT state.

Under load when lots of connections and disconnections happen socket descriptors can stay in TIME_WAIT state. Those descriptors can not be reused for a while. So you can get various
errors when using Centrifugo. For example something like `(99: Cannot assign requested address) while connecting to upstream` in Nginx error log and 502 on client side.

Look how many socket descriptors in TIME_WAIT state.

```
netstat -an |grep TIME_WAIT | grep <CENTRIFUGO_PID> | wc -l
```

Nice article about TIME_WAIT sockets: http://vincent.bernat.im/en/blog/2014-tcp-time-wait-state-linux.html

The advices here are similar to ephemeral port exhaustion problem:

* Increase the ephemeral port range by tuning `ip_local_port_range` option
* Deploy more Centrifugo server instances to load balance across
* Deploy more load balancer instances
* Use virtual network interfaces

### Proxy max connections

Proxies like Nginx and Envoy have default limits on maximum number of connections which can be established.

Make sure you have a reasonable limit for max number of incoming and outgoing connections in your proxy configuration. 

### Conntrack table

More rare (since default limit is usually sufficient) your possible number of connections can be limited by conntrack table. Netfilter framework which is part of iptables keeps information about all connections and has limited size for this information. See how to see its limits and instructions to increase [in this article](https://morganwu277.github.io/2018/05/26/Solve-production-issue-of-nf-conntrack-table-full-dropping-packet/).
