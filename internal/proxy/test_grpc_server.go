package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
)

type proxyGRPCTestServer struct {
	flag string
	opts proxyGRPCTestServerOptions
	proxyproto.UnimplementedCentrifugoProxyServer
}

type proxyGRPCTestServerOptions struct {
	User     string
	ExpireAt int64
	B64Data  string
	Channels []string
}

func newProxyGRPCTestServer(flag string, opts proxyGRPCTestServerOptions) proxyGRPCTestServer {
	return proxyGRPCTestServer{
		flag: flag,
		opts: opts,
	}
}

func (p proxyGRPCTestServer) Connect(ctx context.Context, request *proxyproto.ConnectRequest) (*proxyproto.ConnectResponse, error) {
	switch p.flag {
	case "result":
		return &proxyproto.ConnectResponse{
			Result: &proxyproto.ConnectResult{
				User:     p.opts.User,
				ExpireAt: p.opts.ExpireAt,
				B64Data:  p.opts.B64Data,
			},
		}, nil
	case "subscription":
		return &proxyproto.ConnectResponse{
			Result: &proxyproto.ConnectResult{
				User:     p.opts.User,
				Channels: p.opts.Channels,
			},
		}, nil

	case "subscription error":
		return &proxyproto.ConnectResponse{
			Result: &proxyproto.ConnectResult{
				User:     p.opts.User,
				Channels: p.opts.Channels,
			},
		}, nil
	case "custom disconnect":
		return &proxyproto.ConnectResponse{
			Disconnect: &proxyproto.Disconnect{
				Code:      4000,
				Reason:    "custom disconnect",
				Reconnect: false,
			},
		}, nil
	case "custom error":
		return &proxyproto.ConnectResponse{
			Error: &proxyproto.Error{
				Code:    1000,
				Message: "custom error",
			},
		}, nil
	default:
		return &proxyproto.ConnectResponse{}, nil
	}
}

func (p proxyGRPCTestServer) Refresh(ctx context.Context, request *proxyproto.RefreshRequest) (*proxyproto.RefreshResponse, error) {
	panic("it will be implemented later")
}

func (p proxyGRPCTestServer) Subscribe(ctx context.Context, request *proxyproto.SubscribeRequest) (*proxyproto.SubscribeResponse, error) {
	panic("it will be implemented later")
}

func (p proxyGRPCTestServer) Publish(ctx context.Context, request *proxyproto.PublishRequest) (*proxyproto.PublishResponse, error) {
	switch p.flag {
	case "result":
		return &proxyproto.PublishResponse{
			Result: &proxyproto.PublishResult{
				B64Data: p.opts.B64Data,
			},
		}, nil
	case "skip history":
		return &proxyproto.PublishResponse{
			Result: &proxyproto.PublishResult{
				B64Data:     p.opts.B64Data,
				SkipHistory: true,
			},
		}, nil
	case "custom disconnect":
		return &proxyproto.PublishResponse{
			Disconnect: &proxyproto.Disconnect{
				Code:      4000,
				Reason:    "custom disconnect",
				Reconnect: false,
			},
		}, nil
	case "custom error":
		return &proxyproto.PublishResponse{
			Error: &proxyproto.Error{
				Code:    1000,
				Message: "custom error",
			},
		}, nil
	default:
		return &proxyproto.PublishResponse{}, nil
	}
}

func (p proxyGRPCTestServer) RPC(ctx context.Context, request *proxyproto.RPCRequest) (*proxyproto.RPCResponse, error) {
	panic("it will be implemented later")
}
