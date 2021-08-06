package api

import (
	"context"
	"crypto/subtle"

	. "github.com/centrifugal/centrifugo/v3/internal/apiproto"

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
func RegisterGRPCServerAPI(n *centrifuge.Node, apiExecutor *Executor, server *grpc.Server, config GRPCAPIServiceConfig) error {
	RegisterCentrifugoApiServer(server, newGRPCAPIService(n, apiExecutor, config))
	return nil
}

// grpcAPIService can answer on GRPC API requests.
type grpcAPIService struct {
	UnimplementedCentrifugoApiServer

	config GRPCAPIServiceConfig
	api    *Executor
}

// newGRPCAPIService creates new Service.
func newGRPCAPIService(_ *centrifuge.Node, apiExecutor *Executor, c GRPCAPIServiceConfig) *grpcAPIService {
	return &grpcAPIService{
		config: c,
		api:    apiExecutor,
	}
}

// Publish into channel.
func (s *grpcAPIService) Publish(ctx context.Context, req *PublishRequest) (*PublishResponse, error) {
	return s.api.Publish(ctx, req), nil
}

// Broadcast into channels.
func (s *grpcAPIService) Broadcast(ctx context.Context, req *BroadcastRequest) (*BroadcastResponse, error) {
	return s.api.Broadcast(ctx, req), nil
}

// Subscribe user to a channel.
func (s *grpcAPIService) Subscribe(ctx context.Context, req *SubscribeRequest) (*SubscribeResponse, error) {
	return s.api.Subscribe(ctx, req), nil
}

// Unsubscribe user from channel.
func (s *grpcAPIService) Unsubscribe(ctx context.Context, req *UnsubscribeRequest) (*UnsubscribeResponse, error) {
	return s.api.Unsubscribe(ctx, req), nil
}

// Disconnect user.
func (s *grpcAPIService) Disconnect(ctx context.Context, req *DisconnectRequest) (*DisconnectResponse, error) {
	return s.api.Disconnect(ctx, req), nil
}

// History in channel.
func (s *grpcAPIService) History(ctx context.Context, req *HistoryRequest) (*HistoryResponse, error) {
	return s.api.History(ctx, req), nil
}

// HistoryRemove removes all history information for channel.
func (s *grpcAPIService) HistoryRemove(ctx context.Context, req *HistoryRemoveRequest) (*HistoryRemoveResponse, error) {
	return s.api.HistoryRemove(ctx, req), nil
}

// Presence in channel.
func (s *grpcAPIService) Presence(ctx context.Context, req *PresenceRequest) (*PresenceResponse, error) {
	return s.api.Presence(ctx, req), nil
}

// PresenceStats information for channel.
func (s *grpcAPIService) PresenceStats(ctx context.Context, req *PresenceStatsRequest) (*PresenceStatsResponse, error) {
	return s.api.PresenceStats(ctx, req), nil
}

// Info returns information about Centrifugo state.
func (s *grpcAPIService) Info(ctx context.Context, req *InfoRequest) (*InfoResponse, error) {
	return s.api.Info(ctx, req), nil
}

// RPC can return custom data.
func (s *grpcAPIService) RPC(ctx context.Context, req *RPCRequest) (*RPCResponse, error) {
	return s.api.RPC(ctx, req), nil
}

// Refresh user connection.
func (s *grpcAPIService) Refresh(ctx context.Context, req *RefreshRequest) (*RefreshResponse, error) {
	return s.api.Refresh(ctx, req), nil
}
