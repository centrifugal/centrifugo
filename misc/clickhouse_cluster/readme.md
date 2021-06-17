# Clickhouse Cluster

Clickhouse cluster with 2 shards and 2 replicas built with docker-compose.

## Run

Run single command, and it will copy configs for each node and
run clickhouse cluster `company_cluster` with docker-compose
```sh
make config up
```

## Profiles

- `default` - no password
- `admin` - password `123`

## Test it

Login to clickhouse01 console (first node's ports are mapped to localhost)
```sh
clickhouse-client -h localhost
```

```
cluster_name
database_name
```

Create a test database and table (sharded and replicated)
```sql
CREATE DATABASE company_db ON CLUSTER 'company_cluster';

CREATE TABLE company_db.events ON CLUSTER 'company_cluster' (
    time DateTime,
    uid  Int64,
    type LowCardinality(String)
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{cluster}/{shard}/table', '{replica}')
PARTITION BY toDate(time)
ORDER BY (uid);

CREATE TABLE company_db.events_distr ON CLUSTER 'company_cluster' AS company_db.events
ENGINE = Distributed('company_cluster', company_db, events, uid);
```

Load some data
```sql
INSERT INTO company_db.events_distr VALUES
    ('2020-01-01 10:00:00', 100, 'view'),
    ('2020-01-01 10:05:00', 101, 'view'),
    ('2020-01-01 11:00:00', 100, 'contact'),
    ('2020-01-01 12:10:00', 101, 'view'),
    ('2020-01-02 08:10:00', 100, 'view'),
    ('2020-01-03 13:00:00', 103, 'view');
```

Check data from current shard
```sql
SELECT * FROM company_db.events;
```

Check data from all cluster
```sql
SELECT * FROM company_db.events_distr;
```

Connect:

```
docker run -it --rm --net clickhouse_cluster_default --link clickhouse01:clickhouse-server yandex/clickhouse-client --host clickhouse-server
```
