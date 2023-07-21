#!/bin/bash

set -e

# go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
# go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
# go install github.com/fatih/gomodifytags@v1.13.0
# go install github.com/FZambia/gomodifytype@latest

which protoc
which gomodifytype
which gomodifytags
protoc-gen-go --version
protoc-gen-go-grpc --version

protoc -I ./ \
  proxystream.proto \
  --go_out=. \
  --go-grpc_out=.

echo "Done!"
