#!/usr/bin/env bash
# Prints the Go packages to test, one per line. Usage:
#
#   misc/scripts/test_packages.sh unit|integration|all
#
# integration – packages needing the docker-compose services: they have files
#               behind the integration build tag, or their tests import a client
#               of one of the services (such tests usually skip without it).
# unit        – all other packages, they run without services.
# all         – both.
#
# New packages are picked up automatically.
set -euo pipefail

kind=${1:-all}

case "$kind" in
unit | integration | all) ;;
*)
	echo "unknown kind: $kind (want unit, integration or all)" >&2
	exit 2
	;;
esac

cd "$(dirname "$0")/../.."

# Tests importing a client of a docker-compose service need that service.
service_clients='github.com/jackc/pgx/|github.com/redis/rueidis|github.com/redis/go-redis/|github.com/nats-io/nats.go|github.com/twmb/franz-go/|cloud.google.com/go/pubsub|github.com/aws/aws-sdk-go-v2/service/(sqs|sns)|github.com/ClickHouse/clickhouse-go'

module=$(go list -m)
format='{{.ImportPath}} {{join .GoFiles ","}}|{{join .TestGoFiles ","}}|{{join .XTestGoFiles ","}} {{join .TestImports ","}},{{join .XTestImports ","}}'

# Integration packages either differ with the integration tag, or exist only
# with it, or import a service client from tests.
{
	go list -tags integration -f "$format" ./... | awk '{print $1, $2}'
	go list -f "$format" ./... | awk '{print $1, $2}'
} | sort | uniq -u | awk '{print $1}' | sort -u >"${TMPDIR:-/tmp}/test_packages_tagged.$$"
trap 'rm -f "${TMPDIR:-/tmp}/test_packages_tagged.$$"' EXIT

go list -tags integration -f "$format" ./... |
	grep -v "^$module/misc/" |
	awk -v clients="$service_clients" -v tagged="${TMPDIR:-/tmp}/test_packages_tagged.$$" -v kind="$kind" '
		BEGIN { while ((getline p < tagged) > 0) integration[p] = 1 }
		{
			pkg = $1
			if ($3 ~ clients) integration[pkg] = 1
			if (kind == "all" || (kind == "integration") == (pkg in integration)) print pkg
		}' |
	sort
