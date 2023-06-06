package api

import (
	"context"
	"crypto/subtle"

	. "github.com/centrifugal/centrifugo/v5/internal/apiproto"

	"github.com/centrifugal/centrifuge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func authorize(ctx context.Context, key []byte) error {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if len(md["authorization"]) > 0 && subtle.ConstantTimeCompare([]byte(md["authorization"][0]), key) == 1 {
			return nil
		}
	}
	return status.Error(codes.Unauthenticated, "unauthenticated")
}

// GRPCKeyAuth allows to set simple authentication based on string key from configuration.
// Client should provide per RPC credentials: set authorization key to metadata with value
// `apikey <KEY>`.
func GRPCKeyAuth(key string) grpc.ServerOption {
	authKey := []byte("apikey " + key)
	return grpc.UnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		if err := authorize(ctx, authKey); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	})
}

// GRPCAPIServiceConfig for GRPC API Service.
type GRPCAPIServiceConfig struct{}

// RegisterGRPCServerAPI registers GRPC API service in provided GRPC server.
func RegisterGRPCServerAPI(n *centrifuge.Node, apiExecutor *Executor, server *grpc.Server, config GRPCAPIServiceConfig, useOpenTelemetry bool) error {
	RegisterCentrifugoApiServer(server, newGRPCAPIService(n, apiExecutor, config, useOpenTelemetry))
	return nil
}

// grpcAPIService can answer on GRPC API requests.
type grpcAPIService struct {
	UnimplementedCentrifugoApiServer

	config           GRPCAPIServiceConfig
	api              *Executor
	useOpenTelemetry bool
}

// newGRPCAPIService creates new Service.
func newGRPCAPIService(_ *centrifuge.Node, apiExecutor *Executor, c GRPCAPIServiceConfig, useOpenTelemetry bool) *grpcAPIService {
	return &grpcAPIService{
		config:           c,
		api:              apiExecutor,
		useOpenTelemetry: useOpenTelemetry,
	}
}
