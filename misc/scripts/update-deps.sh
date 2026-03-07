#!/usr/bin/env bash
# Update all Go module dependencies.
#
# Usage:
#   ./scripts/update-deps.sh          # minor/patch updates only
#   ./scripts/update-deps.sh --major  # also apply major version updates
set -euo pipefail

cd "$(pwd)"

pwd

echo "==> Updating all dependencies to latest minor/patch versions..."
go get -u ./...

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
