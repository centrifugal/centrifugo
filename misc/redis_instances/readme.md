Create local N isolated Redis master instances starting from port `6000`:

```
bash start_instances.sh 3
```

Redis nodes will work until script interrupted. The goal of having this script is for development and benchmarking.

By default, Redis runs without RDB and AOF (`--save "" --appendonly no`).
