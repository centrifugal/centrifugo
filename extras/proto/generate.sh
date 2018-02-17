#!/bin/bash

gomplate -f $GOPATH/src/github.com/centrifugal/centrifugo/extras/proto/api.template.proto > $GOPATH/src/github.com/centrifugal/centrifugo/extras/proto/api.proto
GOGO=1 gomplate -f $GOPATH/src/github.com/centrifugal/centrifugo/extras/proto/api.template.proto > $GOPATH/src/github.com/centrifugal/centrifugo/lib/proto/apiproto/api.proto
gomplate -f $GOPATH/src/github.com/centrifugal/centrifugo/extras/proto/client.template.proto > $GOPATH/src/github.com/centrifugal/centrifugo/extras/proto/client.proto
GOGO=1 gomplate -f $GOPATH/src/github.com/centrifugal/centrifugo/extras/proto/client.template.proto > $GOPATH/src/github.com/centrifugal/centrifugo/lib/proto/client.proto
