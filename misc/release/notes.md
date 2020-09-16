This release has backwards incompatible changes in some Prometheus/Graphite metric names. This means that you may need to adapt your monitoring dashboards a bit. See details below.

Improvements:

* Previously metrics exposed by Centrifuge library (which Centrifugo is built on top of) belonged to `centrifuge` Prometheus namespace. This lead to a situation where part of Centrifugo metrics belonged to `centrifugo` and part to `centrifuge` Prometheus namespaces. Starting from v2.7.0 Centrifuge library specific metrics also belong to `centrifugo` namespace. So the rule to migrate is simple: if see `centrifuge` word in a metric name – change it to `centrifugo`. 
* Official Centrifugo Helm Chart for Kubernetes – [check out its repo](https://github.com/centrifugal/helm-charts)
* Refreshed login screen of admin web interface with moving Centrifugo logo on canvas – just check it out!
* New gauge that shows amount of running Centrifugo nodes

Fixes:

* Fix `messages_sent_count` counter which did not show control, join and leave messages

**Coming soon:** official Grafana Dashboard for Prometheus storage is on its way to Centrifugo users. [Track this issue](https://github.com/centrifugal/centrifugo/issues/383) for status, the work almost finished. 
