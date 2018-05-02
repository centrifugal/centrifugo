package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/centrifugal/centrifuge"
	"google.golang.org/grpc"
	// "github.com/grpc-ecosystem/go-grpc-middleware"
	// "google.golang.org/grpc"
)

func handleLog(e centrifuge.LogEntry) {
	log.Printf("%s: %v", e.Message, e.Fields)
}

func authMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		newCtx := centrifuge.SetCredentials(ctx, &centrifuge.Credentials{
			UserID: "42",
			Exp:    time.Now().Unix() + 10,
			Info:   []byte(`{"name": "Alexander"}`),
		})
		r = r.WithContext(newCtx)
		h.ServeHTTP(w, r)
	})
}

func waitExitSignal(n *centrifuge.Node, srv *grpc.Server) {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		n.Shutdown()
		srv.GracefulStop()
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
				Publish:         true,
				Presence:        true,
				JoinLeave:       true,
				HistoryLifetime: 60,
				HistorySize:     10,
			},
		},
	}

	node := centrifuge.New(cfg)

	node.OnConnect(func(ctx context.Context, client centrifuge.Client, e centrifuge.ConnectEvent) centrifuge.ConnectReply {

		client.OnSubscribe(func(e centrifuge.SubscribeEvent) centrifuge.SubscribeReply {
			log.Printf("user %s subscribes on %s", client.UserID(), e.Channel)
			return centrifuge.SubscribeReply{}
		})

		client.OnUnsubscribe(func(e centrifuge.UnsubscribeEvent) centrifuge.UnsubscribeReply {
			log.Printf("user %s unsubscribed from %s", client.UserID(), e.Channel)
			return centrifuge.UnsubscribeReply{}
		})

		client.OnPublish(func(e centrifuge.PublishEvent) centrifuge.PublishReply {
			log.Printf("user %s publishes into channel %s: %s", client.UserID(), e.Channel, string(e.Data))
			return centrifuge.PublishReply{}
		})

		client.OnRPC(func(e centrifuge.RPCEvent) centrifuge.RPCReply {
			log.Printf("RPC from user: %s, data: %s", client.UserID(), string(e.Data))
			return centrifuge.RPCReply{
				Data: []byte(`{"year": "2018"}`),
			}
		})

		client.OnMessage(func(e centrifuge.MessageEvent) centrifuge.MessageReply {
			log.Printf("Message from user: %s, data: %s", client.UserID(), string(e.Data))
			return centrifuge.MessageReply{}
		})

		client.OnRefresh(func(e centrifuge.RefreshEvent) centrifuge.RefreshReply {
			log.Printf("user %s connection is going to expire, refreshing", client.UserID())
			return centrifuge.RefreshReply{
				Exp: time.Now().Unix() + 60,
			}
		})

		client.OnDisconnect(func(e centrifuge.DisconnectEvent) centrifuge.DisconnectReply {
			log.Printf("user %s disconnected, disconnect: %#v", client.UserID(), e.Disconnect)
			return centrifuge.DisconnectReply{}
		})

		log.Printf("user %s connected via %s with encoding: %d", client.UserID(), client.Transport().Name(), client.Transport().Encoding())

		go func() {
			messageData, _ := json.Marshal("hello")
			err := client.Send(messageData)
			if err != nil {
				if err == io.EOF {
					return
				}
				log.Fatalln(err.Error())
			}
		}()

		return centrifuge.ConnectReply{
			Data: []byte(`{"timezone": "Moscow/Europe"}`),
		}
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

	// // Also handle GRPC client connections on :8002.
	// authInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	// 	ctx := ss.Context()
	// 	newCtx := centrifuge.SetCredentials(ctx, &centrifuge.Credentials{
	// 		UserID: "42",
	// 		Exp:    time.Now().Unix() + 10,
	// 		Info:   []byte(`{"name": "Alexander"}`),
	// 	})
	// 	wrapped := grpc_middleware.WrapServerStream(ss)
	// 	wrapped.WrappedContext = newCtx
	// 	return handler(srv, wrapped)
	// }
	// grpcServer := grpc.NewServer(
	// 	grpc.StreamInterceptor(authInterceptor),
	// )
	// centrifuge.RegisterGRPCServerClient(node, grpcServer, centrifuge.GRPCClientServiceConfig{})
	// go func() {
	// 	listener, _ := net.Listen("tcp", ":8001")
	// 	if err := grpcServer.Serve(listener); err != nil {
	// 		logger.FATAL.Fatalf("Serve GRPC: %v", err)
	// 	}
	// }()

	// waitExitSignal(node, grpcServer)
	select {}
	fmt.Println("exiting")
}
