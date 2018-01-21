package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/centrifugal/centrifugo/lib/channel"
	"github.com/centrifugal/centrifugo/lib/client"
	"github.com/centrifugal/centrifugo/lib/engine/enginememory"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"
	"github.com/centrifugal/centrifugo/lib/rpc"
	"github.com/centrifugal/centrifugo/lib/server"
)

func authMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Our middleware logic goes here...
		ctx := r.Context()
		ctx = context.WithValue(ctx, client.CredentialsContextKey, &client.Credentials{UserID: "42"})
		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	})
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

	handleRPC := func(ctx context.Context, req *rpc.Request) (*rpc.Response, *proto.Disconnect) {

		var userID string
		value := ctx.Value(client.CredentialsContextKey)
		credentials, ok := value.(*client.Credentials)
		if ok {
			userID = credentials.UserID
		}

		log.Printf("RPC from user: %s, method: %s, params: %s", userID, req.Method, string(req.Params))
		result := []byte(`{"text": "rpc response"}`)

		go func() {
			err := <-n.Publish("$public:chat", &proto.Publication{Data: []byte(`{"input": "Booom!"}`)}, nil)
			if err != nil {
				log.Fatalln(err)
			}
		}()

		return &rpc.Response{
			Error:  nil,
			Result: result,
		}, &proto.Disconnect{Reason: "go away", Reconnect: false}
	}

	n.SetRPCHandler(handleRPC)

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

	http.Handle("/", authMiddleware(mux))

	go func() {
		if err := http.ListenAndServe(":8000", nil); err != nil {
			panic(err)
		}
	}()

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		n.Shutdown()
		done <- true
	}()

	<-done
	fmt.Println("exiting")
}
