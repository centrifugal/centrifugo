package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/api"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/tools"
	"github.com/centrifugal/centrifugo/v6/internal/unigrpc"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

// grpcServer is a gRPC server with its bound listener.
type grpcServer struct {
	server  *grpc.Server
	ln      net.Listener
	logLine string // logged when it starts serving
	errMsg  string // logged when serving fails
}

// listen binds a gRPC listener and registers a stop step closing it.
func (a *App) listenGRPC(name string, addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("cannot listen to address %s", addr)
	}
	a.lifecycle.OnStop(name+" listener", func(context.Context) error {
		_ = ln.Close()
		return nil
	})
	a.addAddr(ln.Addr())
	return ln, nil
}

// buildGRPCAPIServer builds the gRPC API server and binds its listener.
func (a *App) buildGRPCAPIServer() (*grpcServer, error) {
	cfg := a.cfg
	node := a.node
	useAPIOpentelemetry := cfg.OpenTelemetry.Enabled && cfg.OpenTelemetry.API
	grpcAPIExecutor := a.grpcAPIExecutor
	grpcAPIAddr := net.JoinHostPort(cfg.GrpcAPI.Address, strconv.Itoa(cfg.GrpcAPI.Port))
	var err error
	var grpcOpts []grpc.ServerOption

	// NOTE: not a bug – GRPC API without a key is intentional and documented behaviour in
	// Centrifugo v6, see https://centrifugal.dev/docs/server/server_api. Unlike the HTTP API,
	// GRPC API is disabled by default and runs on its own port, separate from the main HTTP
	// server, so it's expected to be exposed to an internal network only. It may also be
	// protected without a key – by mutual TLS via grpc_api.tls, or by network rules.
	// Do not make an empty key fail-closed here: it silently breaks existing setups. In v7 we
	// may require at least one auth method unless an explicit insecure flag is set.
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
	ln, err := a.listenGRPC("GRPC API", grpcAPIAddr)
	if err != nil {
		return nil, err
	}
	return &grpcServer{
		server:  grpcAPIServer,
		ln:      ln,
		logLine: fmt.Sprintf("serving GRPC API service on %s", grpcAPIAddr),
		errMsg:  "serve GRPC API",
	}, nil
}

// buildGRPCUniServer builds the unidirectional gRPC server and binds its
// listener.
func (a *App) buildGRPCUniServer() (*grpcServer, error) {
	cfg := a.cfg
	node := a.node
	grpcUniAddr := net.JoinHostPort(cfg.UniGRPC.Address, strconv.Itoa(cfg.UniGRPC.Port))
	var err error
	var grpcOpts []grpc.ServerOption
	grpcOpts = append(grpcOpts, grpc.ForceServerCodec(&unigrpc.RawCodec{}))

	if cfg.UniGRPC.MaxReceiveMessageSize > 0 {
		grpcOpts = append(grpcOpts, grpc.MaxRecvMsgSize(cfg.UniGRPC.MaxReceiveMessageSize))
	}

	var uniGrpcTLSConfig *tls.Config
	if cfg.UniGRPC.TLS.Enabled {
		uniGrpcTLSConfig, err = cfg.UniGRPC.TLS.ToGoTLSConfig("uni_grpc")
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
	ln, err := a.listenGRPC("uni GRPC", grpcUniAddr)
	if err != nil {
		return nil, err
	}
	return &grpcServer{
		server:  grpcUniServer,
		ln:      ln,
		logLine: fmt.Sprintf("serving unidirectional GRPC on %s", grpcUniAddr),
		errMsg:  "serve uni GRPC",
	}, nil
}
