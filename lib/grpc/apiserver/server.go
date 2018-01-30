package apiserver

import (
	"context"

	"github.com/centrifugal/centrifugo/lib/api"
	"github.com/centrifugal/centrifugo/lib/node"
	apiproto "github.com/centrifugal/centrifugo/lib/proto/api"
)

// Config for GRPC API server.
type Config struct {
	// APIKey ...
	APIKey string
	// APIInsecure ...
	APIInsecure bool
}

// Server can answer on GRPC API requests.
type Server struct {
	config Config
	api    *api.Handler
}

// New creates new server.
func New(n *node.Node, c Config) *Server {
	return &Server{
		config: c,
		api:    api.NewHandler(n),
	}
}

// Publish into channel.
func (s *Server) Publish(ctx context.Context, req *apiproto.PublishRequest) (*apiproto.PublishResponse, error) {
	return s.api.Publish(ctx, req), nil
}

// Broadcast into channels.
func (s *Server) Broadcast(ctx context.Context, req *apiproto.BroadcastRequest) (*apiproto.BroadcastResponse, error) {
	return s.api.Broadcast(ctx, req), nil
}

// Channels allows to retrive list of channels.
func (s *Server) Channels(ctx context.Context, req *apiproto.ChannelsRequest) (*apiproto.ChannelsResponse, error) {
	return s.api.Channels(ctx, req), nil
}

// Unsubscribe user from channel.
func (s *Server) Unsubscribe(ctx context.Context, req *apiproto.UnsubscribeRequest) (*apiproto.UnsubscribeResponse, error) {
	return s.api.Unsubscribe(ctx, req), nil
}

// Disconnect user.
func (s *Server) Disconnect(ctx context.Context, req *apiproto.DisconnectRequest) (*apiproto.DisconnectResponse, error) {
	return s.api.Disconnect(ctx, req), nil
}

// History in channel.
func (s *Server) History(ctx context.Context, req *apiproto.HistoryRequest) (*apiproto.HistoryResponse, error) {
	return s.api.History(ctx, req), nil
}

// Presence in channel.
func (s *Server) Presence(ctx context.Context, req *apiproto.PresenceRequest) (*apiproto.PresenceResponse, error) {
	return s.api.Presence(ctx, req), nil
}

// PresenceStats information for channel.
func (s *Server) PresenceStats(ctx context.Context, req *apiproto.PresenceStatsRequest) (*apiproto.PresenceStatsResponse, error) {
	return s.api.PresenceStats(ctx, req), nil
}

// Info returns information about Centrifugo state.
func (s *Server) Info(ctx context.Context, req *apiproto.InfoRequest) (*apiproto.InfoResponse, error) {
	return s.api.Info(ctx, req), nil
}
