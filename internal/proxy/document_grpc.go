package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCDocumentProxy ...
type GRPCDocumentProxy struct {
	config Config
	client proxyproto.CentrifugoProxyClient
}

var _ DocumentProxy = (*GRPCDocumentProxy)(nil)

// NewGRPCDocumentProxy ...
func NewGRPCDocumentProxy(p Config) (*GRPCDocumentProxy, error) {
	host, err := getGrpcHost(p.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("error getting grpc host: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.Timeout))
	defer cancel()
	dialOpts, err := getDialOpts(p)
	if err != nil {
		return nil, fmt.Errorf("error creating GRPC dial options: %v", err)
	}
	conn, err := grpc.DialContext(ctx, host, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("error connecting to GRPC proxy server: %v", err)
	}
	return &GRPCDocumentProxy{
		config: p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// Protocol ...
func (p *GRPCDocumentProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCDocumentProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// LoadDocuments ...
func (p *GRPCDocumentProxy) LoadDocuments(ctx context.Context, req *proxyproto.LoadDocumentsRequest) (*proxyproto.LoadDocumentsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.config.Timeout))
	defer cancel()
	return p.client.LoadDocuments(grpcRequestContext(ctx, p.config), req, grpc.ForceCodec(grpcCodec))
}
