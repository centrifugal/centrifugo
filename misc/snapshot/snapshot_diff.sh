#!/usr/bin/env bash
# Builds Centrifugo at BASE (a git ref) and from the current working tree,
# runs both through the snapshot scenarios and diffs the results.
#
#   BASE=<git ref> [SNAPSHOT_FLAGS="..."] [SNAPSHOT_OUT=dir] misc/snapshot/snapshot_diff.sh
#
# The base is run twice first: if those runs differ the snapshot itself is
# not deterministic and the comparison is not trusted. Exit status: 0 no
# difference, 1 difference, 2 snapshot not deterministic or error.
# SNAPSHOT_FLAGS are passed to every run, e.g.
#   SNAPSHOT_FLAGS="-with redis,postgres -scenarios extra/ -config extra.json"
set -euo pipefail

if [ -z "${BASE:-}" ]; then
  echo "BASE is required, e.g. BASE=master" >&2
  exit 2
fi

ROOT=$(git rev-parse --show-toplevel)
SNAPDIR="$ROOT/misc/snapshot"
WORK=$(mktemp -d "${TMPDIR:-/tmp}/snapshot-diff.XXXXXX")
OUT=${SNAPSHOT_OUT:-$WORK/out}
mkdir -p "$OUT"
BASE_TREE="$WORK/base"

cleanup() {
  git -C "$ROOT" worktree remove --force "$BASE_TREE" >/dev/null 2>&1 || true
  rm -f "$WORK/centrifugo-base" "$WORK/centrifugo-head" "$WORK/snapshot"
}
trap cleanup EXIT

echo "building base $BASE ($(git -C "$ROOT" rev-parse --short "$BASE"))"
git -C "$ROOT" worktree add --quiet --detach "$BASE_TREE" "$BASE"
(cd "$BASE_TREE" && go build -o "$WORK/centrifugo-base" .)
echo "building current tree"
(cd "$ROOT" && go build -o "$WORK/centrifugo-head" .)
(cd "$SNAPDIR" && go build -o "$WORK/snapshot" .)

# shellcheck disable=SC2206
FLAGS=(-config "$SNAPDIR/config.json" -scenarios "$SNAPDIR/scenarios" ${SNAPSHOT_FLAGS:-})

start=$(date +%s)
"$WORK/snapshot" -binary "$WORK/centrifugo-base" "${FLAGS[@]}" -out "$OUT/base1"
"$WORK/snapshot" -binary "$WORK/centrifugo-base" "${FLAGS[@]}" -out "$OUT/base2"
if ! "$WORK/snapshot" diff "$OUT/base1" "$OUT/base2" > "$OUT/determinism.diff"; then
  cat "$OUT/determinism.diff"
  echo "snapshot not deterministic: two runs of the base differ (see above, outputs in $OUT)" >&2
  exit 2
fi
"$WORK/snapshot" -binary "$WORK/centrifugo-head" "${FLAGS[@]}" -out "$OUT/head"
echo "snapshot runs took $(( $(date +%s) - start ))s"

if "$WORK/snapshot" diff "$OUT/base1" "$OUT/head" > "$OUT/snapshot.diff"; then
  echo "no difference between $BASE and the current tree"
  [ -n "${SNAPSHOT_OUT:-}" ] || rm -rf "$WORK"
  exit 0
fi
cat "$OUT/snapshot.diff"
echo "snapshot differs between $BASE and the current tree (outputs in $OUT)" >&2
exit 1
