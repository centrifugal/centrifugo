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
	"github.com/centrifugal/centrifugo/lib/httpserver"
	"github.com/centrifugal/centrifugo/lib/logging"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"
)

func handleLog(e logging.Entry) {
	log.Printf("[centrifuge %s] %s: %v", logging.LevelString(e.Level), e.Message, e.Fields)
}

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
		RPC:         handleRPC,
		Connect:     handleConnect,
		Disconnect:  handleDisconnect,
		Subscribe:   handleSubscribe,
		Unsubscribe: handleUnsubscribe,
		Publish:     handlePublish,
	}

	n.SetMediator(mediator)
	n.SetLogHandler(logging.DEBUG, handleLog)

	e, _ := enginememory.New(n, &enginememory.Config{})

	if err := n.Run(e); err != nil {
		panic(err)
	}

	http.Handle("/connection/websocket", authMiddleware(httpserver.NewWebsocketHandler(n, httpserver.WebsocketConfig{})))

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
