package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
			Exp:    time.Now().Unix() + 10,
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
	cfg.ClientExpire = true
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
		log.Printf("RPC from user: %s, data: %s, encoding: %d", req.Client.UserID(), string(req.Data), req.Client.Transport().Encoding())

		return &centrifuge.RPCReply{
			Data: []byte(`{"temperature": "-42"}`),
		}, nil
	}

	handleMessage := func(ctc context.Context, req *centrifuge.MessageContext) (*centrifuge.MessageReply, error) {
		log.Printf("Message from user: %s, data: %s", req.Client.UserID(), string(req.Data))
		return nil, nil
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

	handlePresence := func(ctx context.Context, req *centrifuge.PresenceContext) (*centrifuge.PresenceReply, error) {
		log.Printf("user %s is active and subscribed on channels %#v", req.Client.UserID(), req.Channels)
		return nil, nil
	}

	handleRefresh := func(ctx context.Context, req *centrifuge.RefreshContext) (*centrifuge.RefreshReply, error) {
		log.Printf("user %s connection is going to expire, refresh it", req.Client.UserID())
		return &centrifuge.RefreshReply{
			Exp: time.Now().Unix() + 60,
		}, nil
	}

	mediator := &centrifuge.Mediator{
		RPC:         handleRPC,
		Message:     handleMessage,
		Connect:     handleConnect,
		Disconnect:  handleDisconnect,
		Subscribe:   handleSubscribe,
		Unsubscribe: handleUnsubscribe,
		Publish:     handlePublish,
		Presence:    handlePresence,
		Refresh:     handleRefresh,
	}

	node.SetMediator(mediator)
	node.SetLogHandler(centrifuge.LogLevelDebug, handleLog)

	if err := node.Run(); err != nil {
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
