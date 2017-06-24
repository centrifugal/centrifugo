package main

import (
	"net/http"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/channel"
	"github.com/centrifugal/centrifugo/libcentrifugo/engine/enginememory"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/server"
)

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

	e, err := enginememory.New(n, &enginememory.Config{})
	if err != nil {
		logger.FATAL.Fatalln(err)
	}

	if err := n.Run(e); err != nil {
		logger.FATAL.Fatalln(err)
	}

	serverConfig := &server.Config{}
	s, _ := server.New(n, serverConfig)

	opts := server.DefaultMuxOptions
	opts.Prefix = "/centrifugo"
	mux := server.ServeMux(s, opts)

	http.Handle("/", mux)

	if err := http.ListenAndServe(":8000", nil); err != nil {
		logger.FATAL.Fatalf("Error running HTTP server: %v", err)
	}
}
