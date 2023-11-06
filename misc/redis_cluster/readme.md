Create local Redis cluster consisting of N nodes with M replicas each, ex:

```
bash create_cluster.sh 3 0
```

Cluster nodes will work until script interrupted. The goal of having this script is for development and benchmarking.

All data will be saved to local directory `cluster_data`. By default, Redis runs without RDB and AOF (`--save "" --appendonly no`).
