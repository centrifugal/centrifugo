#!/bin/bash

set -e

# go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
# go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

protoc -I ./ \
  unistream.proto \
  --go_out=. \
  --go-grpc_out=.
