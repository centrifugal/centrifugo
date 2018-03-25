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
	log.Printf("[centrifuge] %s: %v", e.Message, e.Fields)
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
	cfg.ClientExpire = true
	cfg.Namespaces = []centrifuge.ChannelNamespace{
		centrifuge.ChannelNamespace{
			Name: "chat",
			ChannelOptions: centrifuge.ChannelOptions{
				Publish:   true,
				Presence:  true,
				JoinLeave: true,
			},
		},
	}

	node := centrifuge.New(cfg)

	handleRPC := func(ctx context.Context, req centrifuge.RPCContext) centrifuge.RPCReply {
		log.Printf("RPC from user: %s, data: %s, encoding: %d", req.Client.UserID(), string(req.Data), req.Client.Transport().Encoding())
		return centrifuge.RPCReply{
			Data: []byte(`{"year": "2018"}`),
		}
	}

	handleMessage := func(ctc context.Context, req centrifuge.MessageContext) centrifuge.MessageReply {
		log.Printf("Message from user: %s, data: %s", req.Client.UserID(), string(req.Data))
		return centrifuge.MessageReply{}
	}

	handleConnect := func(ctx context.Context, req centrifuge.ConnectContext) centrifuge.ConnectReply {
		log.Printf("user %s connected via %s", req.Client.UserID(), req.Client.Transport().Name())
		return centrifuge.ConnectReply{}
	}

	handleDisconnect := func(ctx context.Context, req centrifuge.DisconnectContext) centrifuge.DisconnectReply {
		log.Printf("user %s disconnected, disconnect: %#v", req.Client.UserID(), req.Disconnect)
		return centrifuge.DisconnectReply{}
	}

	handleSubscribe := func(ctx context.Context, req centrifuge.SubscribeContext) centrifuge.SubscribeReply {
		log.Printf("user %s subscribes on %s", req.Client.UserID(), req.Channel)
		return centrifuge.SubscribeReply{}
	}

	handleUnsubscribe := func(ctx context.Context, req centrifuge.UnsubscribeContext) centrifuge.UnsubscribeReply {
		log.Printf("user %s unsubscribed from %s", req.Client.UserID(), req.Channel)
		return centrifuge.UnsubscribeReply{}
	}

	handlePublish := func(ctx context.Context, req centrifuge.PublishContext) centrifuge.PublishReply {
		log.Printf("user %s publishes into channel %s: %s", req.Client.UserID(), req.Channel, string(req.Publication.Data))
		return centrifuge.PublishReply{}
	}

	handlePresence := func(ctx context.Context, req centrifuge.PresenceContext) centrifuge.PresenceReply {
		log.Printf("user %s is online and subscribed on channels %#v", req.Client.UserID(), req.Channels)
		return centrifuge.PresenceReply{}
	}

	handleRefresh := func(ctx context.Context, req centrifuge.RefreshContext) centrifuge.RefreshReply {
		log.Printf("user %s connection is going to expire, refreshing", req.Client.UserID())
		return centrifuge.RefreshReply{
			Exp: time.Now().Unix() + 60,
		}
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
	http.Handle("/", http.FileServer(http.Dir("./")))

	go func() {
		if err := http.ListenAndServe(":8000", nil); err != nil {
			panic(err)
		}
	}()

	waitExitSignal(node)
	fmt.Println("exiting")
}
