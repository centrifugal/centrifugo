package main

import (
	"context"
	"log"
	"net/http"

	"github.com/centrifugal/centrifugo/lib/channel"
	"github.com/centrifugal/centrifugo/lib/engine/enginememory"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"
	"github.com/centrifugal/centrifugo/lib/server"
)

func handleRPC(ctx context.Context, method string, params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	log.Printf("RPC received, method: %s, params: %s", method, string(params))
	result := []byte(`{"rpc": true}`)
	return nil, nil, nil
}

func main() {
	nodeConfig := node.DefaultConfig
	nodeConfig.Secret = "secret"
	nodeConfig.Namespaces = []channel.Namespace{
		channel.Namespace{
			Name: "public",
			Options: channel.Options{
				Publish:   true,
				Presence:  true,
				JoinLeave: true,
			},
		},
	}
	n := node.New(nodeConfig)
	n.RegisterRPCHandler(handleRPC)

	e, err := enginememory.New(n, &enginememory.Config{})
	if err != nil {
		panic(err)
	}

	if err := n.Run(e); err != nil {
		panic(err)
	}

	serverConfig := &server.Config{}
	s, err := server.New(n, serverConfig)
	if err != nil {
		panic(err)
	}

	opts := server.MuxOptions{
		Prefix:       "", // can be sth like "/centrifugo" in theory
		HandlerFlags: server.HandlerWebsocket,
	}
	mux := server.ServeMux(s, opts)

	http.Handle("/", mux)

	if err := http.ListenAndServe(":8000", nil); err != nil {
		panic(err)
	}
}
