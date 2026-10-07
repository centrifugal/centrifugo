#!/usr/bin/env bash
# Prints the Go packages to test, one per line, so CI can split tests into
# parallel jobs. Usage:
#
#   misc/scripts/test_packages.sh unit|integration|all [SHARD TOTAL]
#
# integration – packages needing the docker-compose services: they have files
#               behind the integration build tag, or their tests import a client
#               of one of the services (such tests usually skip without it).
# unit        – all other packages, they run without services.
# all         – both.
#
# With SHARD and TOTAL (1 <= SHARD <= TOTAL) only the SHARD-th of TOTAL
# disjoint subsets is printed. Packages are assigned greedily by weight (see
# below, unknown packages weigh 1) so shards finish at about the same time. The
# result depends only on the package list, every package lands in exactly one
# shard, and new packages are picked up automatically.
set -euo pipefail

kind=${1:-all}
shard=${2:-1}
total=${3:-1}

case "$kind" in
unit | integration | all) ;;
*)
	echo "unknown kind: $kind (want unit, integration or all)" >&2
	exit 2
	;;
esac
if ! [[ "$shard" =~ ^[0-9]+$ && "$total" =~ ^[0-9]+$ ]] || ((shard < 1 || shard > total)); then
	echo "bad shard $shard of $total" >&2
	exit 2
fi

cd "$(dirname "$0")/../.."

# Relative cost of testing a package (compilation with -race plus run time),
# for packages noticeably heavier than an average one, which weighs 1.
weights='
internal/consuming=6
internal/pgmapbroker=4
internal/pgstreambroker=3
internal/controllers=2
internal/pgoutbox=2
internal/api=2
internal/config=2
internal/jwtverify=2
internal/uniws=2
internal/websocket=2
internal/proxy=2
internal/client=2
'

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
	WEIGHTS="$weights" awk -v module="$module/" -v shard="$shard" -v total="$total" '
		BEGIN {
			n = split(ENVIRON["WEIGHTS"], pairs, /[ \n]+/)
			for (i = 1; i <= n; i++) {
				if (split(pairs[i], f, "=") == 2) w[module f[1]] = f[2]
			}
		}
		{ pkgs[++count] = $1 }
		END {
			# Heaviest first, by name within the same weight: deterministic.
			for (i = 1; i <= count; i++) weight[i] = (pkgs[i] in w) ? w[pkgs[i]] : 1
			for (i = 2; i <= count; i++) {
				p = pkgs[i]; pw = weight[i]
				for (j = i - 1; j >= 1 && (weight[j] < pw || (weight[j] == pw && pkgs[j] > p)); j--) {
					pkgs[j + 1] = pkgs[j]; weight[j + 1] = weight[j]
				}
				pkgs[j + 1] = p; weight[j + 1] = pw
			}
			# Greedy: each package goes to the least loaded shard (lowest index on ties).
			for (s = 1; s <= total; s++) load[s] = 0
			for (i = 1; i <= count; i++) {
				best = 1
				for (s = 2; s <= total; s++) if (load[s] < load[best]) best = s
				load[best] += weight[i]
				if (best == shard) print pkgs[i]
			}
		}' |
	sort
