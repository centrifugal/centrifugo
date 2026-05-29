# Contributing to Centrifugo

Thanks for your interest in improving Centrifugo! This document explains how to
get a development environment running and how to get your change merged.

## Where to ask questions

The issue tracker is for **bug reports** and **feature proposals**. If you have a
usage question or want to discuss an idea before opening an issue, please reach
out in the community channels – you'll usually get an answer much faster:

* [Telegram](https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ)
* [Discord](https://discord.gg/tYgADKx)

Before opening an issue, please check the [documentation](https://centrifugal.dev)
and [FAQ](https://centrifugal.dev/docs/faq), and search existing issues.

## Reporting bugs and proposing features

Open an issue using one of the [issue templates](https://github.com/centrifugal/centrifugo/issues/new/choose).
For bugs, the most helpful reports include the Centrifugo version, your
configuration (with secrets redacted), the client SDK and version, and a minimal
way to reproduce the problem.

For security vulnerabilities, **do not open a public issue** – follow the process
in [SECURITY.md](SECURITY.md).

## Prerequisites

* [Go](https://go.dev/dl/) – use the version specified in [`go.mod`](go.mod) (or newer).
* [Docker](https://docs.docker.com/get-docker/) and Docker Compose – used to run
  Redis, PostgreSQL, Nats, and Kafka for integration tests.
* `make`.

## Building and running locally

Build the binary:

```bash
make build       # or: CGO_ENABLED=0 go build
```

Generate a minimal config and start the server:

```bash
./centrifugo genconfig          # writes config.json
./centrifugo -c config.json
```

See `./centrifugo --help` for the full list of commands and flags, and the
[configuration docs](https://centrifugal.dev/docs/server/configuration) for
details.

## Running tests

Unit tests (with the race detector):

```bash
make test
```

Integration tests additionally exercise Redis, PostgreSQL, Nats, and Kafka, so
start the dependencies first:

```bash
docker compose up -d --wait
make test-integration
```

This mirrors what CI runs (see [`.github/workflows/test.yml`](.github/workflows/test.yml)),
so running it locally is a good way to reproduce CI failures.

## Linting

Centrifugo uses [`golangci-lint`](https://golangci-lint.run/) pinned to the
version in [`.golangci-lint-version`](.golangci-lint-version):

```bash
make install-lint   # installs the pinned version
make lint
```

You can also check for known vulnerabilities in dependencies:

```bash
make vuln           # runs govulncheck
```

## Code generation

Some code is generated from Protobuf definitions and from the configuration
schema. If you change a `.proto` file (under `internal/apiproto`,
`internal/proxyproto`, `internal/unigrpc`) or the configuration types, regenerate
the derived files:

```bash
make generate
```

This requires the Protobuf toolchain (`protoc`/`buf`) on your `PATH`. Commit the
regenerated files together with your change.

## Submitting a pull request

1. Fork the repository and create a topic branch.
2. Keep pull requests focused – one logical change per PR is easier to review.
3. Make sure `make lint` and the relevant tests pass before pushing.
4. If you change behavior, update the documentation and add or adjust tests.
5. Fill in the [pull request template](.github/PULL_REQUEST_TEMPLATE.md) and link
   any related issue.

For larger or behavior-changing work, it's worth opening an issue (or chatting in
the community channels) first, so the design can be discussed before you invest
time in an implementation.

## License

By contributing to Centrifugo, you agree that your contributions will be licensed
under the [Apache 2.0 License](LICENSE).
