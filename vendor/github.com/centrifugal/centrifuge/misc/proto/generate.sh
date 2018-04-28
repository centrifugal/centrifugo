#!/bin/bash
go get github.com/hairyhenderson/gomplate

gomplate -f $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/api.template.proto > $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/api.proto
GOGO=1 gomplate -f $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/api.template.proto > $GOPATH/src/github.com/centrifugal/centrifuge/internal/proto/apiproto/api.proto
gomplate -f $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/client.template.proto > $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/client.proto
GOGO=1 gomplate -f $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/client.template.proto > $GOPATH/src/github.com/centrifugal/centrifuge/internal/proto/client.proto
GOGO=1 gomplate -f $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/client.template.proto > $GOPATH/src/github.com/centrifugal/centrifuge/misc/proto/client.gogo.proto

cd $GOPATH/src/github.com/centrifugal/centrifuge/internal/proto/apiproto && protoc --proto_path=$GOPATH/src:$GOPATH/src/github.com/centrifugal/centrifuge/vendor:. --gogofaster_out=plugins=grpc:. api.proto
cd $GOPATH/src/github.com/centrifugal/centrifuge/internal/proto && protoc --proto_path=$GOPATH/src:$GOPATH/src/github.com/centrifugal/centrifuge/vendor:. --gogofaster_out=plugins=grpc:. client.proto
