#!/bin/sh
# Checks the output of a snapshot release build, see .github/workflows/release-check.yml.
set -eu

fail() {
  echo "release check failed: $*" >&2
  exit 1
}

checksums=$(ls dist/*_checksums.txt)

echo "Archives, SBOMs and packages are in $checksums"
for f in dist/*.tar.gz dist/*.zip PACKAGES/*.deb PACKAGES/*.rpm; do
  name=$(basename "$f")
  grep -q " $name\$" "$checksums" || fail "$name is not in $checksums"
  case "$f" in
    dist/*)
      [ -f "$f.sbom.json" ] || fail "no SBOM for $name"
      grep -q " $name.sbom.json\$" "$checksums" || fail "SBOM of $name is not in $checksums"
      ;;
  esac
done
(cd dist && sha256sum --quiet --ignore-missing -c "$(basename "$checksums")") || fail "checksum mismatch in dist"
(cd PACKAGES && sha256sum --quiet --ignore-missing -c "../$checksums") || fail "checksum mismatch in PACKAGES"

echo "Binary runs"
"$(find dist -path '*linux_amd64*' -type f -name centrifugo | head -1)" version

echo "Docker images run"
images=$(jq -r '.[] | select(.type == "Docker Image" and .goarch == "amd64") | .name' dist/artifacts.json)
[ -n "$images" ] || fail "no Docker images built"
for image in $images; do
  docker run --rm --entrypoint /usr/local/bin/centrifugo "$image" version || fail "$image does not run"
done

echo "release check OK"
