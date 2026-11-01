package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	_ "github.com/centrifugal/centrifugo/v6/internal/proxyproto" // registers proxy proto types.

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ProxyRequest is one request received by the recording proxy.
type ProxyRequest struct {
	Transport string            `json:"transport"`
	Kind      string            `json:"kind"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      any               `json:"body"`
	// Response is what the proxy answered (rule index or "unmatched").
	Response string `json:"response"`
}

// Recorder serves the HTTP and gRPC proxy endpoints.
type Recorder struct {
	mu       sync.Mutex
	rules    []ProxyRule
	requests []ProxyRequest
	notify   chan struct{}

	httpLn  net.Listener
	grpcLn  net.Listener
	httpSrv *http.Server
	grpcSrv *grpc.Server
}

// headers never recorded: they depend on body length, connection handling
// or timing rather than on what Centrifugo sends.
var skipHTTPHeaders = map[string]bool{
	"Content-Length":  true,
	"Accept-Encoding": true,
	"Connection":      true,
	"Date":            true,
}

var skipGRPCMetadata = map[string]bool{
	":authority":   true,
	"grpc-timeout": true,
	"content-type": true,
}

func NewRecorder() (*Recorder, error) {
	r := &Recorder{notify: make(chan struct{}, 1)}
	var err error
	r.httpLn, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	r.grpcLn, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	r.httpSrv = &http.Server{Handler: http.HandlerFunc(r.serveHTTP), ReadHeaderTimeout: 5 * time.Second}
	r.grpcSrv = grpc.NewServer(
		grpc.ForceServerCodecV2(rawCodec{}),
		grpc.UnknownServiceHandler(r.serveGRPC),
	)
	go func() { _ = r.httpSrv.Serve(r.httpLn) }()
	go func() { _ = r.grpcSrv.Serve(r.grpcLn) }()
	return r, nil
}

func (r *Recorder) HTTPPort() int { return r.httpLn.Addr().(*net.TCPAddr).Port }
func (r *Recorder) GRPCPort() int { return r.grpcLn.Addr().(*net.TCPAddr).Port }

func (r *Recorder) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = r.httpSrv.Shutdown(ctx)
	r.grpcSrv.Stop()
}

func (r *Recorder) SetRules(rules []ProxyRule) {
	r.mu.Lock()
	r.rules = rules
	r.mu.Unlock()
}

// Take returns requests received since the previous call.
// asyncKinds are proxy calls the server makes on its own timers (connection
// and subscription expiration), not in response to a step. They are taken
// only by wait_proxy or at the end of the scenario, so their position in the
// transcript does not depend on timing.
var asyncKinds = map[string]bool{"refresh": true, "sub_refresh": true}

// Take returns requests received since the previous call. Unless all is
// set, requests of asyncKinds stay pending.
func (r *Recorder) Take(all bool) []ProxyRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out, keep []ProxyRequest
	for _, req := range r.requests {
		if !all && asyncKinds[req.Kind] {
			keep = append(keep, req)
		} else {
			out = append(out, req)
		}
	}
	r.requests = keep
	return out
}

// TakeKind takes up to count pending requests of kind, oldest first.
func (r *Recorder) TakeKind(kind string, count int) []ProxyRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out, keep []ProxyRequest
	for _, req := range r.requests {
		if req.Kind == kind && len(out) < count {
			out = append(out, req)
		} else {
			keep = append(keep, req)
		}
	}
	r.requests = keep
	return out
}

// WaitKind waits until at least count requests of kind are pending.
func (r *Recorder) WaitKind(kind string, count int, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		r.mu.Lock()
		n := 0
		for _, req := range r.requests {
			if req.Kind == kind {
				n++
			}
		}
		r.mu.Unlock()
		if n >= count {
			return true
		}
		select {
		case <-r.notify:
		case <-deadline:
			return false
		}
	}
}

func (r *Recorder) record(req ProxyRequest) {
	r.mu.Lock()
	r.requests = append(r.requests, req)
	r.mu.Unlock()
	select {
	case r.notify <- struct{}{}:
	default:
	}
}

func (r *Recorder) match(kind string, body any) (int, *ProxyRule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.rules {
		rule := &r.rules[i]
		if rule.Kind != kind {
			continue
		}
		ok := true
		for path, want := range rule.Match {
			got, found := lookupPath(body, path)
			if !found || CanonicalJSON(got) != CanonicalJSON(want) {
				ok = false
				break
			}
		}
		if ok {
			return i, rule
		}
	}
	return -1, nil
}

func lookupPath(v any, path string) (any, bool) {
	for _, part := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return v, true
}

func (r *Recorder) serveHTTP(w http.ResponseWriter, req *http.Request) {
	data, _ := io.ReadAll(req.Body)
	kind := strings.TrimPrefix(req.URL.Path, "/")
	var body any
	if v, err := DecodeJSON(data); err == nil {
		body = v
	} else {
		body = "raw:" + string(data)
	}
	headers := map[string]string{}
	for k, vals := range req.Header {
		if skipHTTPHeaders[k] {
			continue
		}
		headers[k] = strings.Join(vals, ",")
	}
	if req.URL.RawQuery != "" {
		headers[":query"] = req.URL.RawQuery
	}
	headers[":method"] = req.Method
	idx, rule := r.match(kind, body)
	pr := ProxyRequest{Transport: "http", Kind: kind, Headers: headers, Body: body}
	if rule == nil {
		pr.Response = "unmatched"
		r.record(pr)
		http.Error(w, "no scripted response", http.StatusNotImplemented)
		return
	}
	pr.Response = fmt.Sprintf("rule %d", idx)
	r.record(pr)
	out, err := expandJSON(rule.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	st := rule.Status
	if st == 0 {
		st = http.StatusOK
	}
	w.WriteHeader(st)
	_, _ = w.Write(out)
}

func (r *Recorder) serveGRPC(_ any, stream grpc.ServerStream) error {
	fullMethod, _ := grpc.MethodFromServerStream(stream)
	parts := strings.Split(strings.TrimPrefix(fullMethod, "/"), "/")
	if len(parts) != 2 {
		return status.Error(codes.Unimplemented, "bad method")
	}
	kind := snakeCase(parts[1])
	var in []byte
	if err := stream.RecvMsg(&in); err != nil {
		return err
	}
	md, _ := metadata.FromIncomingContext(stream.Context())
	headers := map[string]string{}
	for k, vals := range md {
		if skipGRPCMetadata[k] {
			continue
		}
		headers[k] = strings.Join(vals, ",")
	}
	inType, outType, err := methodTypes(parts[0], parts[1])
	var body any
	if err != nil {
		body = fmt.Sprintf("unknown method: %v", err)
	} else {
		msg := dynamicpb.NewMessage(inType)
		if err := proto.Unmarshal(in, msg); err != nil {
			body = fmt.Sprintf("decode error: %v", err)
		} else {
			js, _ := protojson.MarshalOptions{UseProtoNames: true}.Marshal(msg)
			body, _ = DecodeJSON(js)
		}
	}
	idx, rule := r.match(kind, body)
	pr := ProxyRequest{Transport: "grpc", Kind: kind, Headers: headers, Body: body}
	if rule == nil || err != nil {
		pr.Response = "unmatched"
		r.record(pr)
		return status.Error(codes.Unimplemented, "no scripted response")
	}
	pr.Response = fmt.Sprintf("rule %d", idx)
	r.record(pr)
	if rule.GRPCCode != 0 {
		return status.Error(codes.Code(rule.GRPCCode), rule.GRPCMessage)
	}
	respJSON := rule.GRPCBody
	if len(respJSON) == 0 {
		respJSON = rule.Body
	}
	expanded, err := expandJSON(respJSON)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	out := dynamicpb.NewMessage(outType)
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(expanded, out); err != nil {
		return status.Error(codes.Internal, "scripted gRPC response: "+err.Error())
	}
	b, err := proto.Marshal(out)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	return stream.SendMsg(&b)
}

func methodTypes(service, method string) (protoreflect.MessageDescriptor, protoreflect.MessageDescriptor, error) {
	d, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(service))
	if err != nil {
		return nil, nil, err
	}
	sd, ok := d.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, nil, fmt.Errorf("%s is not a service", service)
	}
	md := sd.Methods().ByName(protoreflect.Name(method))
	if md == nil {
		return nil, nil, fmt.Errorf("no method %s in %s", method, service)
	}
	return md.Input(), md.Output(), nil
}

func snakeCase(s string) string {
	if s == "RPC" {
		return "rpc"
	}
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// rawCodec passes message bytes through; the handler decodes them.
type rawCodec struct{}

func (rawCodec) Marshal(v any) (mem.BufferSlice, error) {
	b, ok := v.(*[]byte)
	if !ok {
		return nil, fmt.Errorf("rawCodec: unexpected %T", v)
	}
	return mem.BufferSlice{mem.SliceBuffer(*b)}, nil
}

func (rawCodec) Unmarshal(data mem.BufferSlice, v any) error {
	b, ok := v.(*[]byte)
	if !ok {
		return fmt.Errorf("rawCodec: unexpected %T", v)
	}
	*b = data.Materialize()
	return nil
}

func (rawCodec) Name() string { return "proto" }

// sortProxyRequests orders requests of one step deterministically.
func sortProxyRequests(reqs []string) {
	sort.Strings(reqs)
}
