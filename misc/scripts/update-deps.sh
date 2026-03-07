#!/usr/bin/env bash
# Update all Go module dependencies.
#
# Usage:
#   ./scripts/update-deps.sh          # minor/patch updates only
#   ./scripts/update-deps.sh --major  # also apply major version updates
set -euo pipefail

cd "$(pwd)"

pwd

# Packages to exclude from automatic updates (updated manually).
EXCLUDE=(
    "github.com/centrifugal/centrifuge"
    "github.com/mailru/easyjson"
)

# Record current versions of excluded packages.
SAVED=()
for pkg in "${EXCLUDE[@]}"; do
    ver=$(grep "^[[:space:]]*${pkg} " go.mod | awk '{print $2}')
    if [[ -n "$ver" ]]; then
        SAVED+=("${pkg}@${ver}")
    fi
done

echo "==> Updating all dependencies to latest minor/patch versions..."
go get -u ./...

# Restore excluded packages to their original versions.
for entry in "${SAVED[@]}"; do
    echo "==> Pinning $entry (excluded from update)"
    go get "$entry"
done

echo "==> Running go mod tidy..."
go mod tidy

if [[ "${1:-}" == "--major" ]]; then
    if ! command -v gomajor &>/dev/null; then
        echo "gomajor not found, installing..."
        go install github.com/icholy/gomajor@latest
    fi
    echo "==> Checking for major version updates..."
    gomajor list
    echo ""
    read -rp "Apply all major version updates? [y/N] " answer
    if [[ "$answer" =~ ^[Yy]$ ]]; then
        gomajor get -a ./...
        echo "==> Running go mod tidy after major updates..."
        go mod tidy
    fi
fi

echo "==> Done. Review changes with: git diff go.mod"
