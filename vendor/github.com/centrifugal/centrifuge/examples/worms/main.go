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

	node.OnConnect(func(ctx context.Context, client centrifuge.Client, e centrifuge.ConnectEvent) centrifuge.ConnectReply {

		client.OnMessage(func(e centrifuge.MessageEvent) centrifuge.MessageReply {
			var ev event
			_ = json.Unmarshal(e.Data, &ev)
			node.Publish("moving", &centrifuge.Pub{Data: []byte(ev.Payload)})
			return centrifuge.MessageReply{}
		})

		client.OnDisconnect(func(e centrifuge.DisconnectEvent) centrifuge.DisconnectReply {
			log.Printf("worm disconnected, disconnect: %#v", e.Disconnect)
			return centrifuge.DisconnectReply{}
		})

		client.OnSubscribe(func(e centrifuge.SubscribeEvent) centrifuge.SubscribeReply {
			log.Printf("worm subscribed on %s", e.Channel)
			return centrifuge.SubscribeReply{}
		})

		client.OnUnsubscribe(func(e centrifuge.UnsubscribeEvent) centrifuge.UnsubscribeReply {
			log.Printf("worm unsubscribed from %s", e.Channel)
			return centrifuge.UnsubscribeReply{}
		})

		log.Printf("worm connected via %s", client.Transport().Name())
		return centrifuge.ConnectReply{}
	})

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
