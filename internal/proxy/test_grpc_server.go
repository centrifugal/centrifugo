package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
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
	Data     []byte
}

func newProxyGRPCTestServer(flag string, opts proxyGRPCTestServerOptions) proxyGRPCTestServer {
	return proxyGRPCTestServer{
		flag: flag,
		opts: opts,
	}
}

func (p proxyGRPCTestServer) Connect(_ context.Context, _ *proxyproto.ConnectRequest) (*proxyproto.ConnectResponse, error) {
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
			Disconnect: p.newDisconnect(),
		}, nil
	case "custom error":
		return &proxyproto.ConnectResponse{
			Error: p.newCustomError(),
		}, nil
	default:
		return &proxyproto.ConnectResponse{}, nil
	}
}

func (p proxyGRPCTestServer) Refresh(_ context.Context, _ *proxyproto.RefreshRequest) (*proxyproto.RefreshResponse, error) {
	switch p.flag {
	case "with credentials":
		return &proxyproto.RefreshResponse{
			Result: &proxyproto.RefreshResult{
				B64Info:  p.opts.B64Data,
				ExpireAt: p.opts.ExpireAt,
			},
		}, nil
	case "expired":
		return &proxyproto.RefreshResponse{
			Result: &proxyproto.RefreshResult{
				Expired: true,
			},
		}, nil
	default:
		return &proxyproto.RefreshResponse{}, nil
	}
}

func (p proxyGRPCTestServer) Subscribe(_ context.Context, _ *proxyproto.SubscribeRequest) (*proxyproto.SubscribeResponse, error) {
	switch p.flag {
	case "result":
		return &proxyproto.SubscribeResponse{
			Result: &proxyproto.SubscribeResult{
				B64Info: p.opts.B64Data,
				B64Data: p.opts.B64Data,
			},
		}, nil
	case "override":
		return &proxyproto.SubscribeResponse{
			Result: &proxyproto.SubscribeResult{
				B64Info: p.opts.B64Data,
				Override: &proxyproto.SubscribeOptionOverride{
					Presence:           &proxyproto.BoolValue{Value: true},
					JoinLeave:          &proxyproto.BoolValue{Value: false},
					ForcePushJoinLeave: &proxyproto.BoolValue{Value: false},
					ForcePositioning:   &proxyproto.BoolValue{Value: true},
					ForceRecovery:      &proxyproto.BoolValue{Value: true},
				},
			},
		}, nil
	case "custom disconnect":
		return &proxyproto.SubscribeResponse{
			Disconnect: p.newDisconnect(),
		}, nil
	case "custom error":
		return &proxyproto.SubscribeResponse{
			Error: p.newCustomError(),
		}, nil
	default:
		return &proxyproto.SubscribeResponse{}, nil
	}
}

func (p proxyGRPCTestServer) Publish(_ context.Context, _ *proxyproto.PublishRequest) (*proxyproto.PublishResponse, error) {
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
			Disconnect: p.newDisconnect(),
		}, nil
	case "custom error":
		return &proxyproto.PublishResponse{
			Error: p.newCustomError(),
		}, nil
	default:
		return &proxyproto.PublishResponse{}, nil
	}
}

func (p proxyGRPCTestServer) RPC(_ context.Context, _ *proxyproto.RPCRequest) (*proxyproto.RPCResponse, error) {
	switch p.flag {
	case "result":
		return &proxyproto.RPCResponse{
			Result: &proxyproto.RPCResult{
				Data: p.opts.Data,
			},
		}, nil
	case "custom disconnect":
		return &proxyproto.RPCResponse{
			Disconnect: p.newDisconnect(),
		}, nil
	case "custom error":
		return &proxyproto.RPCResponse{
			Error: p.newCustomError(),
		}, nil
	case "custom data":
		return &proxyproto.RPCResponse{
			Result: &proxyproto.RPCResult{
				B64Data: p.opts.B64Data,
			},
		}, nil
	default:
		return &proxyproto.RPCResponse{}, nil
	}
}

func (p proxyGRPCTestServer) newDisconnect() *proxyproto.Disconnect {
	return &proxyproto.Disconnect{
		Code:   4000,
		Reason: "custom disconnect",
	}
}

func (p proxyGRPCTestServer) newCustomError() *proxyproto.Error {
	return &proxyproto.Error{
		Code:    1000,
		Message: "custom error",
	}
}
