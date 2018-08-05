package api

import (
	"context"

	"github.com/centrifugal/centrifuge"
	"google.golang.org/grpc"
)

// GRPCAPIServiceConfig for GRPC API Service.
type GRPCAPIServiceConfig struct{}

// RegisterGRPCServerAPI registers GRPC API service in provided GRPC server.
func RegisterGRPCServerAPI(n *centrifuge.Node, server *grpc.Server, config GRPCAPIServiceConfig) error {
	RegisterCentrifugeServer(server, newGRPCAPIService(n, config))
	return nil
}

// grpcAPIService can answer on GRPC API requests.
type grpcAPIService struct {
	config GRPCAPIServiceConfig
	api    *apiExecutor
}

// newGRPCAPIService creates new Service.
func newGRPCAPIService(n *centrifuge.Node, c GRPCAPIServiceConfig) *grpcAPIService {
	return &grpcAPIService{
		config: c,
		api:    newAPIExecutor(n, "grpc"),
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

// Channels allows to retrive list of channels.
func (s *grpcAPIService) Channels(ctx context.Context, req *ChannelsRequest) (*ChannelsResponse, error) {
	return s.api.Channels(ctx, req), nil
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
