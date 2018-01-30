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
	"github.com/centrifugal/centrifugo/lib/events"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"
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

	handleRPC := func(ctx context.Context, req *events.RPCContext) (*events.RPCReply, error) {

		var userID string
		value := ctx.Value(client.CredentialsContextKey)
		credentials, ok := value.(*client.Credentials)
		if ok {
			userID = credentials.UserID
		}

		log.Printf("RPC from user: %s, data: %s", userID, string(req.Data))
		result := []byte(`{"text": "rpc response"}`)

		go func() {
			err := <-n.Publish("$public:chat", &proto.Publication{Data: []byte(`{"input": "Booom!"}`)}, nil)
			if err != nil {
				log.Fatalln(err)
			}
		}()

		return &events.RPCReply{
			Result: result,
		}, nil
	}

	handleConnect := func(ctx context.Context, req *events.ConnectContext) (*events.ConnectReply, error) {
		log.Printf("user %s connected via %s", req.Client.UserID(), req.Client.Transport().Name())
		return nil, nil
	}

	handleDisconnect := func(ctx context.Context, req *events.DisconnectContext) (*events.DisconnectReply, error) {
		log.Printf("user %s disconnected", req.Client.UserID())
		return nil, nil
	}

	handleSubscribe := func(ctx context.Context, req *events.SubscribeContext) (*events.SubscribeReply, error) {
		log.Printf("user %s subscribes on %s", req.Client.UserID(), req.Channel)
		return nil, nil
	}

	handleUnsubscribe := func(ctx context.Context, req *events.UnsubscribeContext) (*events.UnsubscribeReply, error) {
		log.Printf("user %s unsubscribed from %s", req.Client.UserID(), req.Channel)
		return nil, nil
	}

	handlePublish := func(ctx context.Context, req *events.PublishContext) (*events.PublishReply, error) {
		log.Printf("user %s publishes into channel %s: %s", req.Client.UserID(), req.Channel, string(req.Publication.Data))
		return nil, nil
	}

	mediator := &events.Mediator{
		RPCHandler:         handleRPC,
		ConnectHandler:     handleConnect,
		DisconnectHandler:  handleDisconnect,
		SubscribeHandler:   handleSubscribe,
		UnsubscribeHandler: handleUnsubscribe,
		PublishHandler:     handlePublish,
	}

	n.SetMediator(mediator)

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
