package proxyproto

import (
	"context"
)

type ProxyGRPCTestServer struct {
	flag string
	opts ProxyGRPCTestServerOptions
}

type ProxyGRPCTestServerOptions struct {
	User     string
	ExpireAt int64
	B64Data  string
	Channels []string
}

func NewProxyGRPCTestServer(flag string, opts ProxyGRPCTestServerOptions) *ProxyGRPCTestServer {
	return &ProxyGRPCTestServer{
		flag: flag,
		opts: opts,
	}
}

func (p *ProxyGRPCTestServer) Connect(ctx context.Context, request *ConnectRequest) (*ConnectResponse, error) {
	switch p.flag {
	case "result":
		return &ConnectResponse{
			Result: &ConnectResult{
				User:     p.opts.User,
				ExpireAt: p.opts.ExpireAt,
				B64Data:  p.opts.B64Data,
			},
		}, nil
	case "subscription":
		return &ConnectResponse{
			Result: &ConnectResult{
				User:     p.opts.User,
				Channels: p.opts.Channels,
			},
		}, nil

	case "subscription error":
		return &ConnectResponse{
			Result: &ConnectResult{
				User:     p.opts.User,
				Channels: p.opts.Channels,
			},
		}, nil
	case "custom disconnect":
		return &ConnectResponse{
			Disconnect: &Disconnect{
				Code:      4000,
				Reason:    "custom disconnect",
				Reconnect: false,
			},
		}, nil
	case "custom error":
		return &ConnectResponse{
			Error: &Error{
				Code:    1000,
				Message: "custom error",
			},
		}, nil
	default:
		return &ConnectResponse{}, nil
	}
}

func (p *ProxyGRPCTestServer) Refresh(ctx context.Context, request *RefreshRequest) (*RefreshResponse, error) {
	panic("it will be implemented later")
}

func (p *ProxyGRPCTestServer) Subscribe(ctx context.Context, request *SubscribeRequest) (*SubscribeResponse, error) {
	panic("it will be implemented later")
}

func (p *ProxyGRPCTestServer) Publish(ctx context.Context, request *PublishRequest) (*PublishResponse, error) {
	panic("it will be implemented later")
}

func (p *ProxyGRPCTestServer) RPC(ctx context.Context, request *RPCRequest) (*RPCResponse, error) {
	panic("it will be implemented later")
}

func (p *ProxyGRPCTestServer) mustEmbedUnimplementedCentrifugoProxyServer() {
	panic("it will be implemented later")
}
