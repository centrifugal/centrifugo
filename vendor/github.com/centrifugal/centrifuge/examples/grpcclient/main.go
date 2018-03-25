package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"log"
	"sync/atomic"

	"github.com/centrifugal/centrifuge/internal/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

var (
	addr    = flag.String("addr", "localhost:8001", "Server address, e.g. centrifuge.io:443")
	useTLS  = flag.Bool("tls", false, "Use TLS")
	cert    = flag.String("cert", "", "CA certificate file")
	channel = flag.String("channel", "index", "channel to subscribe")
)

func init() {
	log.SetFlags(log.Lshortfile | log.Ldate)
}

var msgID int64

func nextID() int64 {
	return atomic.AddInt64(&msgID, 1)
}

// Disconnect ...
type Disconnect struct {
	Reason    string `json:"reason"`
	Reconnect bool   `json:"reconnect"`
}

func extractDisconnect(md metadata.MD) *Disconnect {
	if value, ok := md["disconnect"]; ok {
		if len(value) > 0 {
			d := value[0]
			var disconnect Disconnect
			err := json.Unmarshal([]byte(d), &disconnect)
			if err == nil {
				return &disconnect
			}
		}
	}
	return &Disconnect{
		Reason:    "connection closed",
		Reconnect: true,
	}
}

// very naive Centrifugo GRPC client example.
func run() {

	var opts []grpc.DialOption
	if *useTLS && *cert != "" {
		// openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout ./server.key -out ./server.cert
		cred, err := credentials.NewClientTLSFromFile(*cert, "")
		if err != nil {
			log.Fatal(err)
		}
		opts = append(opts, grpc.WithTransportCredentials(cred))
	} else if *useTLS {
		creds := credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}

	conn, err := grpc.Dial(*addr, opts...)
	if err != nil {
		log.Fatalf("failed to connect: %s", err)
	}
	defer conn.Close()

	cl := proto.NewCentrifugeClient(conn)

	stream, err := cl.Communicate(context.Background())
	if err != nil {
		log.Println(err)
		return
	}

	err = stream.Send(&proto.Command{
		ID:     uint32(nextID()),
		Method: proto.MethodTypeConnect,
	})

	if err != nil {
		log.Println(err)
		return
	}

	rep, err := stream.Recv()

	if err != nil {
		log.Printf("%#v\n", extractDisconnect(stream.Trailer()))
		log.Println(err)
		return
	}

	var connectResult proto.ConnectResult
	err = connectResult.Unmarshal(rep.Result)
	if err != nil {
		log.Println(err)
		return
	}

	log.Printf("%#v\n", connectResult)

	subscribeRequest := &proto.SubscribeRequest{
		Channel: *channel,
	}

	params, _ := subscribeRequest.Marshal()

	err = stream.Send(&proto.Command{
		ID:     uint32(nextID()),
		Method: proto.MethodTypeSubscribe,
		Params: params,
	})

	rep, err = stream.Recv()

	if err != nil {
		log.Printf("%#v\n", extractDisconnect(stream.Trailer()))
		log.Println(err)
		return
	}

	var subscribeResult proto.SubscribeResult
	err = subscribeResult.Unmarshal(rep.Result)
	if err != nil {
		log.Println(err)
		return
	}

	log.Printf("%#v\n", subscribeResult)

	for {
		rep, err := stream.Recv()
		if err != nil {
			log.Printf("%#v\n", extractDisconnect(stream.Trailer()))
			log.Println(err)
			return
		}
		if rep.ID == 0 {
			var message proto.Message
			err = message.Unmarshal(rep.Result)
			if err != nil {
				log.Println(err)
				return
			}
			if message.Type == proto.MessageTypePublication {
				var publication proto.Publication
				err = publication.Unmarshal(message.Data)
				if err != nil {
					log.Println(err)
					return
				}
				log.Printf("%#v with data: %s\n", publication, string(publication.Data))
			}
		}
	}
}

func main() {
	flag.Parse()
	run()
}
