package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/centrifugal/centrifuge"
)

type event struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

func handleLog(e centrifuge.LogEntry) {
	log.Printf("[centrifuge %s] %s: %v", centrifuge.LogLevelToString(e.Level), e.Message, e.Fields)
}

func authMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Our middleware logic goes here...
		ctx := r.Context()
		ctx = context.WithValue(ctx, centrifuge.CredentialsContextKey, &centrifuge.Credentials{
			UserID: "",
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
	cfg.Anonymous = true

	node := centrifuge.New(cfg)

	handleMessage := func(ctx context.Context, req centrifuge.MessageContext) centrifuge.MessageReply {
		var e event
		_ = json.Unmarshal(req.Data, &e)
		node.Publish("moving", &centrifuge.Publication{Data: []byte(e.Payload)})
		return centrifuge.MessageReply{}
	}

	handleConnect := func(ctx context.Context, req centrifuge.ConnectContext) centrifuge.ConnectReply {
		log.Printf("worm connected via %s", req.Client.Transport().Name())
		return centrifuge.ConnectReply{}
	}

	handleDisconnect := func(ctx context.Context, req centrifuge.DisconnectContext) centrifuge.DisconnectReply {
		log.Printf("worm disconnected, disconnect: %#v", req.Disconnect)
		return centrifuge.DisconnectReply{}
	}

	handleSubscribe := func(ctx context.Context, req centrifuge.SubscribeContext) centrifuge.SubscribeReply {
		log.Printf("worm subscribed on %s", req.Channel)
		return centrifuge.SubscribeReply{}
	}

	handleUnsubscribe := func(ctx context.Context, req centrifuge.UnsubscribeContext) centrifuge.UnsubscribeReply {
		log.Printf("worm unsubscribed from %s", req.Channel)
		return centrifuge.UnsubscribeReply{}
	}

	mediator := &centrifuge.Mediator{
		Message:     handleMessage,
		Connect:     handleConnect,
		Disconnect:  handleDisconnect,
		Subscribe:   handleSubscribe,
		Unsubscribe: handleUnsubscribe,
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
