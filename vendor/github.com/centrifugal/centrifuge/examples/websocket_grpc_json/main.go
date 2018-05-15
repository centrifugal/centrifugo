package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/centrifugal/centrifuge"
	"google.golang.org/grpc"
)

func handleLog(e centrifuge.LogEntry) {
	log.Printf("%s: %v", e.Message, e.Fields)
}

func httpAuthMiddleware(h http.Handler) http.Handler {
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

// Also handle GRPC client connections on :8001.
func grpcAuthInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	// You probably want to authenticate user by information included in stream metadata.
	// meta, ok := metadata.FromIncomingContext(ss.Context())
	// But here we skip it for simplicity and just always authenticate user with ID 42.
	ctx := ss.Context()
	newCtx := centrifuge.SetCredentials(ctx, &centrifuge.Credentials{
		UserID: "42",
		Exp:    time.Now().Unix() + 10,
		Info:   []byte(`{"name": "Alexander"}`),
	})

	// GRPC has no builtin method to add data to context so here we use small
	// wrapper over ServerStream.
	wrapped := WrapServerStream(ss)
	wrapped.WrappedContext = newCtx
	return handler(srv, wrapped)
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

	node, _ := centrifuge.New(cfg)

	node.On().Connect(func(ctx context.Context, client *centrifuge.Client, e centrifuge.ConnectEvent) centrifuge.ConnectReply {

		client.On().Subscribe(func(e centrifuge.SubscribeEvent) centrifuge.SubscribeReply {
			log.Printf("user %s subscribes on %s", client.UserID(), e.Channel)
			return centrifuge.SubscribeReply{}
		})

		client.On().Unsubscribe(func(e centrifuge.UnsubscribeEvent) centrifuge.UnsubscribeReply {
			log.Printf("user %s unsubscribed from %s", client.UserID(), e.Channel)
			return centrifuge.UnsubscribeReply{}
		})

		client.On().Publish(func(e centrifuge.PublishEvent) centrifuge.PublishReply {
			log.Printf("user %s publishes into channel %s: %s", client.UserID(), e.Channel, string(e.Data))
			return centrifuge.PublishReply{}
		})

		client.On().RPC(func(e centrifuge.RPCEvent) centrifuge.RPCReply {
			log.Printf("RPC from user: %s, data: %s", client.UserID(), string(e.Data))
			return centrifuge.RPCReply{
				Data: []byte(`{"year": "2018"}`),
			}
		})

		client.On().Message(func(e centrifuge.MessageEvent) centrifuge.MessageReply {
			log.Printf("Message from user: %s, data: %s", client.UserID(), string(e.Data))
			return centrifuge.MessageReply{}
		})

		client.On().Refresh(func(e centrifuge.RefreshEvent) centrifuge.RefreshReply {
			log.Printf("user %s connection is going to expire, refreshing", client.UserID())
			return centrifuge.RefreshReply{
				Exp: time.Now().Unix() + 60,
			}
		})

		client.On().Disconnect(func(e centrifuge.DisconnectEvent) centrifuge.DisconnectReply {
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

	http.Handle("/connection/websocket", httpAuthMiddleware(centrifuge.NewWebsocketHandler(node, centrifuge.WebsocketConfig{})))
	http.Handle("/", http.FileServer(http.Dir("./")))

	go func() {
		if err := http.ListenAndServe(":8000", nil); err != nil {
			panic(err)
		}
	}()

	grpcServer := grpc.NewServer(
		grpc.StreamInterceptor(grpcAuthInterceptor),
	)
	centrifuge.RegisterGRPCServerClient(node, grpcServer, centrifuge.GRPCClientServiceConfig{})
	go func() {
		listener, _ := net.Listen("tcp", ":8001")
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("Serve GRPC: %v", err)
		}
	}()

	waitExitSignal(node, grpcServer)
	fmt.Println("exiting")
}

// WrappedServerStream is a thin wrapper around grpc.ServerStream that allows modifying context.
// This can be replaced to analogue from github.com/grpc-ecosystem/go-grpc-middleware package -
// https://github.com/grpc-ecosystem/go-grpc-middleware/blob/master/wrappers.go –
// you most probably will have dependency to it in your application as it has lots of useful
// features to deal with GRPC.
type WrappedServerStream struct {
	grpc.ServerStream
	// WrappedContext is the wrapper's own Context. You can assign it.
	WrappedContext context.Context
}

// Context returns the wrapper's WrappedContext, overwriting the nested grpc.ServerStream.Context()
func (w *WrappedServerStream) Context() context.Context {
	return w.WrappedContext
}

// WrapServerStream returns a ServerStream that has the ability to overwrite context.
func WrapServerStream(stream grpc.ServerStream) *WrappedServerStream {
	if existing, ok := stream.(*WrappedServerStream); ok {
		return existing
	}
	return &WrappedServerStream{ServerStream: stream, WrappedContext: stream.Context()}
}
