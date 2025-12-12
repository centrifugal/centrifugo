#!/bin/bash

set -e
set -o pipefail

# Optional: Uncomment if you want commands printed as they run
# set -x

# brew install protobuf
# go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
# go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
# go install github.com/fatih/gomodifytags@v1.13.0
# go install github.com/FZambia/gomodifytype@latest

# List of required tools
REQUIRED_TOOLS=("protoc" "protoc-gen-go" "protoc-gen-go-grpc" "gomodifytype" "gomodifytags")

# Check that each tool exists in PATH
for tool in "${REQUIRED_TOOLS[@]}"; do
    if ! command -v "$tool" &>/dev/null; then
        echo "Error: '$tool' not found in PATH. Please install it before running this script."
        exit 1
    fi
done

echo "Generating Go code from proto..."
protoc -I ./ \
    api.proto \
    --go_out=. \
    --go-grpc_out=.

echo "Modifying types in generated Go code..."
gomodifytype -file api.pb.go -all -w -from "[]byte" -to "Raw"

echo "Generating tags..."
bash generate_tags.sh
