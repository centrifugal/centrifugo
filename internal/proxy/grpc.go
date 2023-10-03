package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/centrifugal/centrifugo/v5/internal/middleware"
	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var grpcCodec = proxyproto.Codec{}

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

func getGrpcHost(endpoint string) (string, error) {
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

func getDialOpts(p Config) ([]grpc.DialOption, error) {
	var dialOpts []grpc.DialOption
	if p.GrpcCredentialsKey != "" {
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(&rpcCredentials{
			key:   p.GrpcCredentialsKey,
			value: p.GrpcCredentialsValue,
		}))
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

	if p.testGrpcDialer != nil {
		dialOpts = append(dialOpts, grpc.WithContextDialer(p.testGrpcDialer))
	}

	return dialOpts, nil
}

func grpcRequestContext(ctx context.Context, proxy Config) context.Context {
	md := requestMetadata(ctx, proxy.HttpHeaders, proxy.GrpcMetadata)
	return metadata.NewOutgoingContext(ctx, md)
}

func httpRequestHeaders(ctx context.Context, proxy Config) http.Header {
	return requestHeaders(ctx, proxy.HttpHeaders, proxy.GrpcMetadata, proxy.StaticHttpHeaders)
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

func requestHeaders(ctx context.Context, allowedHeaders []string, allowedMetaKeys []string, staticHeaders map[string]string) http.Header {
	if headers, ok := middleware.GetHeadersFromContext(ctx); ok {
		return getProxyHeader(headers, allowedHeaders, staticHeaders)
	}
	headers := http.Header{}
	for k, v := range staticHeaders {
		headers.Set(k, v)
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
