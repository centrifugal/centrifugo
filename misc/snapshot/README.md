# snapshot

Runs a Centrifugo binary through scripted scenarios and writes what clients,
backends and operators can observe as normalized text files: client replies
and pushes, requests the proxies receive, server API responses, `/metrics`,
the route table, log lines, exit codes and CLI output. Two builds are compared
with a plain diff, so a refactoring that changes nothing visible produces an
empty diff.

It is a separate Go module (client libraries stay out of Centrifugo's
`go.mod`); it reads the API and proxy protobuf types from the tree it is
built in.

## Running

```
make snapshot-diff BASE=master                                  # memory engines
make snapshot-diff BASE=master SNAPSHOT_FLAGS="-with redis,postgres"
```

`snapshot-diff` builds `BASE` (in a temporary `git worktree`) and the current
working tree, runs the base twice and fails with `snapshot not deterministic`
if those runs differ, then runs the current tree and prints a unified diff.
Exit status: 0 no difference, 1 difference, 2 not deterministic or error.
Set `SNAPSHOT_OUT=dir` to keep the outputs. `make snapshot-test` runs the
tool's own tests. The `snapshot` GitHub workflow runs `snapshot-diff` with
Redis and Postgres on pull requests labeled `snapshot`.

Direct use:

```
cd misc/snapshot && go build -o /tmp/snapshot .
/tmp/snapshot -binary ./centrifugo -config config.json -scenarios scenarios -out /tmp/snap
/tmp/snapshot diff /tmp/snapA /tmp/snapB
```

Flags: `-config` (repeatable; later files are JSON merge patches over earlier
ones), `-scenarios` (repeatable; a later file with the same name replaces an
earlier one), `-out`, `-with redis,postgres,...` (integration services
available; scenarios requiring others are skipped), `-only <regexp>`,
`-timeout` (each waiting step), `-parallel`, and `-redis`, `-postgres`,
`-kafka`, `-nats` addresses for templates. Extra scenarios and config can be
added with more `-scenarios`/`-config` flags (through `SNAPSHOT_FLAGS`)
without changing the tool.

Integration services are the ones from the repository `docker-compose.yml`
(Redis 6379, Postgres 5432 `test:test`).

## Output

One directory per scenario (`<name>` or `<name>@<variant>`):

- `transcript.txt`: every step and what it observed, in step order;
- `logs.txt`: normalized log lines as a sorted, counted multiset (goroutine
  order is not deterministic), split at the shutdown signal; after the
  signal only info and above are kept (debug lines go through an
  asynchronous handler the process may exit before draining);
- `metrics.txt`: Prometheus metrics: names, label names and values, and
  values where the scenario determines them (not for `go_*`, `process_*`,
  histogram buckets and sums, quantiles, `*_seconds`, `*_bytes`, `*_time`);
- `exec-NN-*.txt`: exit code, stdout, stderr and files of CLI commands.

## Normalization

UUIDs become `<uuid-N>` numbered by first appearance in the transcript
(`<uuid>` in logs and metrics), so identities stay comparable. Ports the tool
allocates become `<port:name>`, other loopback ports `<port>`, the run's
temporary directory `<tmp>`, the default node name `<node-name>`, JWTs
`<jwt>`, timestamps `<time>`, fractional or sub-second durations
`<duration>`. Values of the JSON keys `epoch`, `time`, `timestamp`, `uptime`,
`pid`, `gomaxprocs`, `runtime`, `exp`, `iat`, `expire_at`, `ttl`, `elapsed`,
`duration`, `latency`, `took`, `since`, `server_time` become `<key>`. CLI
output keeps durations (they are config values there) and replaces only the Go
version.

## Scenario format

A scenario is a JSON file; files run in name order, each with a fresh server
on free ports. Files are Go templates first, with `{{.Port.http}}` (also
`internal`, `grpc_api`, `uni_grpc`, `proxy_http`, `proxy_grpc`),
`{{.ProxyHTTP}}`, `{{.ProxyGRPC}}`, `{{.TmpDir}}`, `{{.Unique}}` (per run,
for Redis prefixes and table names), `{{.RedisAddr}}`, `{{.PostgresDSN}}`,
`{{.KafkaAddr}}`, `{{.NatsURL}}`.

```json
{
  "description": "shown at the top of the transcript",
  "server": true,
  "requires": ["postgres"],
  "config": {"...": "merge patch over the base config"},
  "env": {"CENTRIFUGO_X": "y"},
  "proxy": [{"kind": "connect", "match": {"name": "app"}, "body": {"result": {"user": "u"}}}],
  "variants": {"memory": {}, "redis": {"requires": ["redis"], "config": {}, "env": {}}},
  "setup": [], "steps": [], "teardown": []
}
```

Config patches follow JSON merge patch rules: objects merge, arrays (such as
`channel.namespaces`) are replaced. Ports are set by the tool; a patch may set
`http_server.internal_port` to `""` to serve everything on one port.

Values in commands, API bodies and proxy answers may use `"$now"`/`"$now+N"`
(unix seconds), `{"$jwt": {claims}, "secret": "..."}` (HS256 token, default
secret `snapshot-secret` as in `config.json`) and `{"$b64": "text"}`.

Steps (`op`), each may have a `comment`:

| op | fields | records |
|---|---|---|
| `connect` | `client`, `headers`, `path`, `query`, `port` | WebSocket dial result |
| `send` | `client`, `cmd`, `no_reply` | command and its reply, or the close code |
| `pushes` | `client`, `count`, `sort` | the next `count` pushes |
| `wait_close` / `close` | `client` | server close code / pending pushes |
| `uni` | `client`, `transport` (`uni_http_stream`, `uni_sse`, `uni_websocket`, `uni_grpc`), `body` | status; messages become pushes |
| `api` | `method` or `path`, `body` or `raw_body`, `auth` (`key`, `bad`, `none`, or a header value), `headers` | HTTP status and response |
| `grpc_api` | `method` (e.g. `Publish`), `body` (protojson), `auth`, `metadata` | response or status code |
| `http` | `method`, `port` (`external`, `internal`), `path`, `body`, `headers`, `replace` | status, content type, body |
| `routes` | `paths`, `methods`, `port`, `headers` | status and content type per path, method and port |
| `metrics` | `filter` (regexp on names) | `metrics.txt` (scrapes until two scrapes agree) |
| `wait_proxy` | `kind`, `count` | requests the server sends on its own timers (`refresh`, `sub_refresh`) |
| `proxy_rules` | `rules` | replaces the scripted proxy answers |
| `wait_log` | `match` (regexps on the message) | the matching log line |
| `reload` | `config` (merge patch over the current config) or `raw_config`, `match` | config written, SIGHUP, the outcome log line |
| `sql` | `exec`, `dsn` | ok or error |
| `exec` | `args`, `files`, `read_files`, `stdin`, `replace` | `exec-NN-*.txt` |
| `shutdown` | `signal` (`SIGTERM`, `SIGINT`) | exit code; clients' disconnect pushes and close codes |

Without an explicit `shutdown` the server gets SIGTERM after the last step.

Proxies: endpoints in config point at the recording proxy. HTTP requests are
identified by path (`{{.ProxyHTTP}}/connect` has kind `connect`), gRPC
requests by method name in snake case. The first rule with the request's kind
whose `match` (dotted paths into the request JSON) fits answers it, with
`status` and `body` over HTTP and `grpc_body` (or `body`) or
`grpc_code`/`grpc_message` over gRPC; unmatched requests get an error and are
recorded as `unmatched`. Requests are recorded with headers (except
`Content-Length`, `Accept-Encoding`, `Connection`, `Date`) or gRPC metadata
(except `:authority`, `grpc-timeout`, `content-type`), sorted within a step.

## Writing deterministic scenarios

- Consume asynchronous pushes with `pushes` before the step that depends on
  them; pushes left at the end are recorded too.
- Use `wait_proxy` for timer-driven proxy calls; the connection and
  subscription checks run on `client.presence_update_interval` and
  `client.expired_sub_close_delay`, so scenarios lower them.
- Avoid output whose order the server does not define (concurrent batch
  replies to one channel, startup failures racing other goroutines).
- Run twice and diff (`snapshot-diff` does) after adding a scenario.

Other consumers (Kafka, NATS JetStream, Redis streams) follow the Postgres
outbox scenario: a `setup` creating the topic or stream, a step producing
messages (a new op using the service client), `pushes` on a subscribed
client. The consumer must be subscribed before the first message is
produced, which for Kafka means waiting for the group join (`wait_log`).
