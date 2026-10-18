#!/bin/sh
# Signs or verifies every Docker image and manifest published by GoReleaser.
# Several tags share one digest (vX.Y.Z, vX.Y, vX, latest), so each unique
# digest is processed once instead of once per tag (as docker_signs would do).
# Usage: cosign_images.sh sign|verify [extra cosign flags]
set -e

mode=$1
shift
case "$mode" in
  sign|verify) ;;
  *) echo "usage: $0 sign|verify [cosign flags]" >&2; exit 1 ;;
esac

images=$(jq -r '.[] | select(.type == "Published Docker Image" or .type == "Docker Manifest") | .name' dist/artifacts.json |
  while read -r ref; do
    digest=$(docker buildx imagetools inspect "$ref" --format '{{json .Manifest}}' | jq -r .digest)
    echo "${ref%:*}@${digest}"
  done | sort -u)

[ -n "$images" ] || { echo "no published images found" >&2; exit 1; }

for image in $images; do
  echo "$mode $image"
  if [ "$mode" = sign ]; then
    cosign sign --yes "$@" "$image"
  else
    cosign verify "$@" "$image" > /dev/null
  fi
done
