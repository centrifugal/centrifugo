package proxy

import (
	"context"
	"fmt"
	"net/http"

	"github.com/centrifugal/centrifugo/v3/internal/middleware"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

type rpcCredentials struct {
	key   string
	value string
}

func (t rpcCredentials) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{
		t.key: t.value,
	}, nil
}

func (t rpcCredentials) RequireTransportSecurity() bool {
	return false
}

func getDialOpts(c Config) ([]grpc.DialOption, error) {
	var dialOpts []grpc.DialOption
	if c.GRPCConfig.Insecure {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}
	if c.GRPCConfig.CredentialsKey != "" {
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(&rpcCredentials{
			key:   c.GRPCConfig.CredentialsKey,
			value: c.GRPCConfig.CredentialsValue,
		}))
	}
	if c.GRPCConfig.CertFile != "" {
		cred, err := credentials.NewClientTLSFromFile(c.GRPCConfig.CertFile, "")
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS credentials %v", err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(cred))
	}
	dialOpts = append(dialOpts, grpc.WithBlock())
	return dialOpts, nil
}

func grpcRequestContext(ctx context.Context, config Config) context.Context {
	md := requestMetadata(ctx, config.HTTPHeaders, config.GRPCMetadata)
	return metadata.NewOutgoingContext(ctx, md)
}

func httpRequestHeaders(ctx context.Context, config Config) http.Header {
	return requestHeaders(ctx, config.HTTPHeaders, config.GRPCMetadata)
}

func requestMetadata(ctx context.Context, allowedHeaders []string, allowedMetaKeys []string) metadata.MD {
	requestMD := metadata.MD{}
	if headers, ok := middleware.GetHeadersFromContext(ctx); ok {
		for k, vv := range headers {
			if stringInSlice(k, allowedHeaders) {
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

func requestHeaders(ctx context.Context, allowedHeaders []string, allowedMetaKeys []string) http.Header {
	headers := http.Header{}
	if headers, ok := middleware.GetHeadersFromContext(ctx); ok {
		return getProxyHeader(headers, allowedHeaders)
	}
	headers.Set("Content-Type", "application/json")
	md, _ := metadata.FromIncomingContext(ctx)
	for k, vv := range md {
		if stringInSlice(k, allowedMetaKeys) {
			headers[k] = vv
		}
	}
	return headers
}
