package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/centrifugal/centrifuge"
)

func handleLog(e centrifuge.LogEntry) {
	log.Printf("[centrifuge %s] %s: %v", centrifuge.LogLevelToString(e.Level), e.Message, e.Fields)
}

func authMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Our middleware logic goes here...
		ctx := r.Context()
		ctx = context.WithValue(ctx, centrifuge.CredentialsContextKey, &centrifuge.Credentials{
			UserID: "42",
			Info:   []byte(`{"name": "Alexander"}`),
		})
		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	})
}

func waitExitSignal(n *centrifuge.Node) {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		n.Shutdown()
		done <- true
	}()
	<-done
}

func main() {
	cfg := centrifuge.DefaultConfig
	cfg.Secret = "secret"
	cfg.Namespaces = []centrifuge.ChannelNamespace{
		centrifuge.ChannelNamespace{
			Name: "public",
			ChannelOptions: centrifuge.ChannelOptions{
				Publish:   true,
				Presence:  true,
				JoinLeave: true,
			},
		},
	}

	node := centrifuge.New(cfg)

	handleRPC := func(ctx context.Context, req *centrifuge.RPCContext) (*centrifuge.RPCReply, error) {

		var userID string
		value := ctx.Value(centrifuge.CredentialsContextKey)
		credentials, ok := value.(*centrifuge.Credentials)
		if ok {
			userID = credentials.UserID
		}

		log.Printf("RPC from user: %s, data: %s, encoding: %d", userID, string(req.Data), req.Client.Transport().Encoding())
		data := []byte(`{"text": "rpc response"}`)

		go func() {
			err := node.Publish("$public:chat", &centrifuge.Publication{Data: []byte(`{"input": "Booom!"}`)})
			if err != nil {
				log.Fatalln(err)
			}
		}()

		return &centrifuge.RPCReply{
			Data: data,
		}, nil
	}

	handleConnect := func(ctx context.Context, req *centrifuge.ConnectContext) (*centrifuge.ConnectReply, error) {
		log.Printf("user %s connected via %s", req.Client.UserID(), req.Client.Transport().Name())
		return nil, nil
	}

	handleDisconnect := func(ctx context.Context, req *centrifuge.DisconnectContext) (*centrifuge.DisconnectReply, error) {
		log.Printf("user %s disconnected", req.Client.UserID())
		return nil, nil
	}

	handleSubscribe := func(ctx context.Context, req *centrifuge.SubscribeContext) (*centrifuge.SubscribeReply, error) {
		log.Printf("user %s subscribes on %s", req.Client.UserID(), req.Channel)
		return nil, nil
	}

	handleUnsubscribe := func(ctx context.Context, req *centrifuge.UnsubscribeContext) (*centrifuge.UnsubscribeReply, error) {
		log.Printf("user %s unsubscribed from %s", req.Client.UserID(), req.Channel)
		return nil, nil
	}

	handlePublish := func(ctx context.Context, req *centrifuge.PublishContext) (*centrifuge.PublishReply, error) {
		log.Printf("user %s publishes into channel %s: %s", req.Client.UserID(), req.Channel, string(req.Publication.Data))
		return nil, nil
	}

	mediator := &centrifuge.Mediator{
		RPC:         handleRPC,
		Connect:     handleConnect,
		Disconnect:  handleDisconnect,
		Subscribe:   handleSubscribe,
		Unsubscribe: handleUnsubscribe,
		Publish:     handlePublish,
	}

	node.SetMediator(mediator)
	node.SetLogHandler(centrifuge.LogLevelDebug, handleLog)

	engine, _ := centrifuge.NewMemoryEngine(node, centrifuge.MemoryEngineConfig{})
	if err := node.Run(engine); err != nil {
		panic(err)
	}

	http.Handle("/connection/websocket", authMiddleware(centrifuge.NewWebsocketHandler(node, centrifuge.WebsocketConfig{})))

	go func() {
		if err := http.ListenAndServe(":8000", nil); err != nil {
			panic(err)
		}
	}()

	waitExitSignal(node)
	fmt.Println("exiting")
}
