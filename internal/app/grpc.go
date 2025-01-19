package app

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/api"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/tools"
	"github.com/centrifugal/centrifugo/v6/internal/unigrpc"

	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

func runGRPCAPIServer(cfg config.Config, node *centrifuge.Node, useAPIOpentelemetry bool, grpcAPIExecutor *api.Executor) (*grpc.Server, error) {
	grpcAPIAddr := net.JoinHostPort(cfg.GrpcAPI.Address, strconv.Itoa(cfg.GrpcAPI.Port))
	grpcAPIConn, err := net.Listen("tcp", grpcAPIAddr)
	if err != nil {
		return nil, fmt.Errorf("cannot listen to address %s", grpcAPIAddr)
	}
	var grpcOpts []grpc.ServerOption

	if cfg.GrpcAPI.Key != "" {
		grpcOpts = append(grpcOpts, api.GRPCKeyAuth(cfg.GrpcAPI.Key))
	}
	if cfg.GrpcAPI.MaxReceiveMessageSize > 0 {
		grpcOpts = append(grpcOpts, grpc.MaxRecvMsgSize(cfg.GrpcAPI.MaxReceiveMessageSize))
	}
	var grpcAPITLSConfig *tls.Config
	if cfg.GrpcAPI.TLS.Enabled {
		grpcAPITLSConfig, err = cfg.GrpcAPI.TLS.ToGoTLSConfig("grpc_api")
		if err != nil {
			return nil, fmt.Errorf("error getting TLS config for GRPC API: %v", err)
		}
	}
	if grpcAPITLSConfig != nil {
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(grpcAPITLSConfig)))
	}
	if cfg.OpenTelemetry.Enabled && cfg.OpenTelemetry.API {
		grpcOpts = append(grpcOpts, grpc.StatsHandler(otelgrpc.NewServerHandler()))
	}
	grpcErrorMode, err := tools.OptionalStringChoice(cfg.GrpcAPI.ErrorMode, []string{config.TransportErrorMode})
	if err != nil {
		return nil, fmt.Errorf("can't extract grpc error mode: %v", err)
	}
	grpcAPIServer := grpc.NewServer(grpcOpts...)
	_ = api.RegisterGRPCServerAPI(node, grpcAPIExecutor, grpcAPIServer, api.GRPCAPIServiceConfig{
		UseOpenTelemetry:      useAPIOpentelemetry,
		UseTransportErrorMode: grpcErrorMode == config.TransportErrorMode,
	})
	if cfg.GrpcAPI.Reflection {
		reflection.Register(grpcAPIServer)
	}
	log.Info().Msgf("serving GRPC API service on %s", grpcAPIAddr)
	go func() {
		if err := grpcAPIServer.Serve(grpcAPIConn); err != nil {
			log.Fatal().Err(err).Msg("serve GRPC API")
		}
	}()
	return grpcAPIServer, nil
}

func runGRPCUniServer(cfg config.Config, node *centrifuge.Node) (*grpc.Server, error) {
	grpcUniAddr := net.JoinHostPort(cfg.UniGRPC.Address, strconv.Itoa(cfg.UniGRPC.Port))
	grpcUniConn, err := net.Listen("tcp", grpcUniAddr)
	if err != nil {
		return nil, fmt.Errorf("cannot listen to address %s", grpcUniAddr)
	}
	var grpcOpts []grpc.ServerOption
	grpcOpts = append(grpcOpts, grpc.ForceServerCodec(&unigrpc.RawCodec{}))

	if cfg.UniGRPC.MaxReceiveMessageSize > 0 {
		grpcOpts = append(grpcOpts, grpc.MaxRecvMsgSize(cfg.UniGRPC.MaxReceiveMessageSize))
	}

	var uniGrpcTLSConfig *tls.Config
	if cfg.GrpcAPI.TLS.Enabled {
		uniGrpcTLSConfig, err = cfg.GrpcAPI.TLS.ToGoTLSConfig("uni_grpc")
		if err != nil {
			return nil, fmt.Errorf("error getting TLS config for uni GRPC: %v", err)
		}
	}
	if uniGrpcTLSConfig != nil {
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(uniGrpcTLSConfig)))
	}
	keepAliveEnforcementPolicy := keepalive.EnforcementPolicy{
		MinTime: 5 * time.Second,
	}
	keepAliveServerParams := keepalive.ServerParameters{
		Time:    25 * time.Second,
		Timeout: 5 * time.Second,
	}
	grpcOpts = append(grpcOpts, grpc.KeepaliveEnforcementPolicy(keepAliveEnforcementPolicy))
	grpcOpts = append(grpcOpts, grpc.KeepaliveParams(keepAliveServerParams))
	grpcUniServer := grpc.NewServer(grpcOpts...)
	_ = unigrpc.RegisterService(grpcUniServer, unigrpc.NewService(node, cfg.UniGRPC))
	log.Info().Msgf("serving unidirectional GRPC on %s", grpcUniAddr)
	go func() {
		if err := grpcUniServer.Serve(grpcUniConn); err != nil {
			log.Fatal().Err(err).Msg("serve uni GRPC")
		}
	}()
	return grpcUniServer, nil
}
