package main

import (
	"context"
	"fmt"

	_ "github.com/centrifugal/centrifugo/v6/internal/apiproto" // registers API proto types.

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/dynamicpb"
)

const apiServiceName = "centrifugal.centrifugo.api.CentrifugoApi"

// grpcAPI calls a unary method of the server gRPC API. The request is the
// protojson of the method's request message; the response is recorded as
// protojson, or the status code and message on error.
func (r *run) grpcAPI(st Step) error {
	inType, outType, err := methodTypes(apiServiceName, st.Method)
	if err != nil {
		return err
	}
	body, err := expandJSON(st.Body)
	if err != nil {
		return err
	}
	in := dynamicpb.NewMessage(inType)
	if err := protojson.Unmarshal(body, in); err != nil {
		return fmt.Errorf("request: %w", err)
	}
	conn, err := grpc.NewClient(r.addr("grpc_api"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), r.opts.Timeout)
	defer cancel()
	md := metadata.MD{}
	switch st.Auth {
	case "", "key":
		md.Set("authorization", "apikey "+r.apiKey("grpc_api.key"))
	case "bad":
		md.Set("authorization", "apikey bad-key")
	case "none":
	default:
		md.Set("authorization", st.Auth)
	}
	for k, v := range st.Metadata {
		md.Set(k, v)
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	r.line("grpc_api " + st.Method + " " + r.norm.NormalizeJSONBytes(body))
	out := dynamicpb.NewMessage(outType)
	err = conn.Invoke(ctx, "/"+apiServiceName+"/"+st.Method, in, out)
	if err != nil {
		s, _ := status.FromError(err)
		r.line(fmt.Sprintf("grpc_api status %s %q", s.Code(), r.norm.String(s.Message())))
		return nil
	}
	js, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(out)
	if err != nil {
		return err
	}
	r.line("grpc_api resp " + r.norm.NormalizeJSONBytes(js))
	return nil
}
