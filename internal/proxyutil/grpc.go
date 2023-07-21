package proxyutil

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/centrifugal/centrifugo/v5/internal/middleware"
	"google.golang.org/grpc/metadata"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func GetGrpcHost(endpoint string) (string, error) {
	var host string
	if strings.HasPrefix(endpoint, "grpc://") {
		u, err := url.Parse(endpoint)
		if err != nil {
			return "", err
		}
		host = u.Host
	} else {
		host = endpoint
	}
	return host, nil
}

type DialConfig struct {
	// GrpcCertFile is a path to GRPC cert file on disk.
	GrpcCertFile string `mapstructure:"grpc_cert_file" json:"grpc_cert_file,omitempty"`

	PerRPCCredentials credentials.PerRPCCredentials

	TestGrpcDialer func(context.Context, string) (net.Conn, error)
}

func GetDialOpts(p DialConfig) ([]grpc.DialOption, error) {
	var dialOpts []grpc.DialOption
	if p.PerRPCCredentials != nil {
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(p.PerRPCCredentials))
	}
	if p.GrpcCertFile != "" {
		cred, err := credentials.NewClientTLSFromFile(p.GrpcCertFile, "")
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS credentials %v", err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(cred))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	if p.TestGrpcDialer != nil {
		dialOpts = append(dialOpts, grpc.WithContextDialer(p.TestGrpcDialer))
	}
	return dialOpts, nil
}

func GrpcRequestContext(ctx context.Context, httpHeaders []string, grpcMetadata []string) context.Context {
	md := requestMetadata(ctx, httpHeaders, grpcMetadata)
	return metadata.NewOutgoingContext(ctx, md)
}

func requestMetadata(ctx context.Context, allowedHeaders []string, allowedMetaKeys []string) metadata.MD {
	requestMD := metadata.MD{}
	if headers, ok := middleware.GetHeadersFromContext(ctx); ok {
		for k, vv := range headers {
			if stringInSlice(strings.ToLower(k), allowedHeaders) {
				requestMD.Set(k, vv...)
			}
		}
		return requestMD
	}
	md, _ := metadata.FromIncomingContext(ctx)
	for k, vv := range md {
		if stringInSlice(k, allowedMetaKeys) {
			requestMD[k] = vv
		}
	}
	return requestMD
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
