package centrifuge

import (
	"context"

	"github.com/centrifugal/centrifuge/internal/proto/apiproto"
	"google.golang.org/grpc"
)

// GRPCAPIServiceConfig for GRPC API Service.
type GRPCAPIServiceConfig struct{}

// RegisterGRPCServerAPI ...
func RegisterGRPCServerAPI(n *Node, server *grpc.Server, config GRPCAPIServiceConfig) error {
	apiproto.RegisterCentrifugeServer(server, newGRPCAPIService(n, config))
	return nil
}

// grpcAPIService can answer on GRPC API requests.
type grpcAPIService struct {
	config GRPCAPIServiceConfig
	api    *apiExecutor
}

// newGRPCAPIService creates new Service.
func newGRPCAPIService(n *Node, c GRPCAPIServiceConfig) *grpcAPIService {
	return &grpcAPIService{
		config: c,
		api:    newAPIExecutor(n),
	}
}

// Publish into channel.
func (s *grpcAPIService) Publish(ctx context.Context, req *apiproto.PublishRequest) (*apiproto.PublishResponse, error) {
	return s.api.Publish(ctx, req), nil
}

// Broadcast into channels.
func (s *grpcAPIService) Broadcast(ctx context.Context, req *apiproto.BroadcastRequest) (*apiproto.BroadcastResponse, error) {
	return s.api.Broadcast(ctx, req), nil
}

// Channels allows to retrive list of channels.
func (s *grpcAPIService) Channels(ctx context.Context, req *apiproto.ChannelsRequest) (*apiproto.ChannelsResponse, error) {
	return s.api.Channels(ctx, req), nil
}

// Unsubscribe user from channel.
func (s *grpcAPIService) Unsubscribe(ctx context.Context, req *apiproto.UnsubscribeRequest) (*apiproto.UnsubscribeResponse, error) {
	return s.api.Unsubscribe(ctx, req), nil
}

// Disconnect user.
func (s *grpcAPIService) Disconnect(ctx context.Context, req *apiproto.DisconnectRequest) (*apiproto.DisconnectResponse, error) {
	return s.api.Disconnect(ctx, req), nil
}

// History in channel.
func (s *grpcAPIService) History(ctx context.Context, req *apiproto.HistoryRequest) (*apiproto.HistoryResponse, error) {
	return s.api.History(ctx, req), nil
}

// HistoryRemove removes all history information for channel.
func (s *grpcAPIService) HistoryRemove(ctx context.Context, req *apiproto.HistoryRemoveRequest) (*apiproto.HistoryRemoveResponse, error) {
	return s.api.HistoryRemove(ctx, req), nil
}

// Presence in channel.
func (s *grpcAPIService) Presence(ctx context.Context, req *apiproto.PresenceRequest) (*apiproto.PresenceResponse, error) {
	return s.api.Presence(ctx, req), nil
}

// PresenceStats information for channel.
func (s *grpcAPIService) PresenceStats(ctx context.Context, req *apiproto.PresenceStatsRequest) (*apiproto.PresenceStatsResponse, error) {
	return s.api.PresenceStats(ctx, req), nil
}

// Info returns information about Centrifugo state.
func (s *grpcAPIService) Info(ctx context.Context, req *apiproto.InfoRequest) (*apiproto.InfoResponse, error) {
	return s.api.Info(ctx, req), nil
}
