#!/bin/sh
# Checks run before anything is built or published, so that a release does not
# stop half-way because of a wrong tag, invalid credentials or an unavailable
# service. All checks are read-only, except the Sigstore check which signs a
# throwaway file (one extra transparency log entry).
#
# Environment:
#   TAG                 release tag, e.g. v6.10.1
#   RELEASE_REPO        GitHub repository the release is published to
#   PUBLISH_REPOS       GitHub repositories GoReleaser writes to (release, Homebrew tap)
#   GITHUB_TOKEN        token GoReleaser uses for publishing
#   SIGNING_IDENTITY    expected certificate identity of the release workflow (optional)
#   PACKAGECLOUD_REPO   packagecloud repository, e.g. user/repo (optional)
#   PACKAGECLOUD_TOKEN  packagecloud API token (optional)
#   BUF_TOKEN           Buf Schema Registry token (optional)
set -eu

fail() {
  echo "preflight failed: $*" >&2
  exit 1
}

echo "Release tag"
echo "$TAG" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' || fail "tag $TAG is not a release version (vX.Y.Z)"

echo "GitHub access"
for repo in $PUBLISH_REPOS; do
  info=$(curl -fsS -H "Authorization: Bearer $GITHUB_TOKEN" "https://api.github.com/repos/$repo") ||
    fail "can not access $repo with the release token"
  [ "$(echo "$info" | jq -r .permissions.push)" = true ] || fail "release token has no push access to $repo"
done
# Only published releases are returned by tag, a draft from a previous attempt is replaced.
status=$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $GITHUB_TOKEN" \
  "https://api.github.com/repos/$RELEASE_REPO/releases/tags/$TAG")
case "$status" in
  404) ;;
  200) fail "release $TAG is already published in $RELEASE_REPO" ;;
  *) fail "can not check release $TAG in $RELEASE_REPO (HTTP $status)" ;;
esac

if [ -n "${SIGNING_IDENTITY:-}" ]; then
  echo "Sigstore signing"
  dir=$(mktemp -d)
  echo "release preflight $TAG" > "$dir/preflight.txt"
  cosign sign-blob --yes --bundle "$dir/preflight.txt.sigstore.json" "$dir/preflight.txt" > /dev/null ||
    fail "can not sign with Sigstore"
  cosign verify-blob --certificate-identity "$SIGNING_IDENTITY" \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --bundle "$dir/preflight.txt.sigstore.json" "$dir/preflight.txt" ||
    fail "signature does not match identity $SIGNING_IDENTITY"
  rm -rf "$dir"
fi

if [ -n "${PACKAGECLOUD_REPO:-}" ]; then
  echo "packagecloud access"
  curl -fsS -o /dev/null -u "$PACKAGECLOUD_TOKEN:" "https://packagecloud.io/api/v1/repos/$PACKAGECLOUD_REPO.json" ||
    fail "can not access packagecloud repository $PACKAGECLOUD_REPO"
fi

if [ -n "${BUF_TOKEN:-}" ]; then
  echo "Buf Schema Registry access"
  buf registry whoami || fail "can not authenticate to Buf Schema Registry"
fi

echo "preflight OK"
