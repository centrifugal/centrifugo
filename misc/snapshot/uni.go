package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	_ "github.com/centrifugal/centrifugo/v6/internal/unigrpc/unistream" // registers uni stream proto types.

	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/dynamicpb"
)

const uniStreamService = "centrifugal.centrifugo.unistream.CentrifugoUniStream"

// uniClient is a unidirectional connection: every message it receives is
// recorded as a push.
type uniClient struct {
	msgBuffer
	cancel func()
}

func (c *uniClient) close() { c.cancel() }

// uniConnect opens a unidirectional connection with st.Body as the connect
// request. Transport is uni_http_stream (default), uni_sse, uni_websocket
// or uni_grpc.
func (r *run) uniConnect(st Step) error {
	body, err := expandJSON(st.Body)
	if err != nil {
		return err
	}
	c := &uniClient{msgBuffer: newMsgBuffer()}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	transport := orDefault(st.Transport, "uni_http_stream")
	path := orDefault(st.Path, "/connection/"+transport)
	r.line(fmt.Sprintf("uni %s %s", transport, r.norm.NormalizeJSONBytes(body)))
	switch transport {
	case "uni_http_stream", "uni_sse":
		var req *http.Request
		if transport == "uni_sse" {
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL(st.Port)+path+"?cf_connect="+url.QueryEscape(string(body)), nil)
		} else {
			req, err = http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL(st.Port)+path, bytes.NewReader(body))
		}
		if err != nil {
			cancel()
			return err
		}
		for k, v := range st.Headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req) //nolint:bodyclose // closed by readLines.
		if err != nil {
			cancel()
			r.line("uni error: " + r.norm.String(err.Error()))
			return nil
		}
		r.line(fmt.Sprintf("uni status %d content-type=%q", resp.StatusCode, resp.Header.Get("Content-Type")))
		go c.readLines(resp.Body, transport == "uni_sse")
	case "uni_websocket":
		h := http.Header{}
		for k, v := range st.Headers {
			h.Set(k, v)
		}
		d := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
		ws, resp, err := d.Dial(wsURL(r.baseURL(st.Port), path), h)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err != nil {
			cancel()
			code := 0
			if resp != nil {
				code = resp.StatusCode
			}
			r.line(fmt.Sprintf("dial failed: status=%d", code))
			return nil
		}
		c.cancel = func() { cancel(); _ = ws.Close() }
		if err := ws.WriteMessage(websocket.TextMessage, body); err != nil {
			c.cancel()
			return err
		}
		go func() {
			for {
				_, data, err := ws.ReadMessage()
				if err != nil {
					ci := &closeInfo{Code: -1, Reason: "read error"}
					if ce, ok := err.(*websocket.CloseError); ok {
						ci = &closeInfo{Code: ce.Code, Reason: ce.Text}
					}
					c.setClosed(ci)
					return
				}
				for _, line := range bytes.Split(data, []byte("\n")) {
					if len(bytes.TrimSpace(line)) > 0 {
						c.addMessage(decodeOrRaw(line))
					}
				}
			}
		}()
	case "uni_grpc":
		inType, outType, err := methodTypes(uniStreamService, "Consume")
		if err != nil {
			cancel()
			return err
		}
		in := dynamicpb.NewMessage(inType)
		if err := protojson.Unmarshal(body, in); err != nil {
			cancel()
			return err
		}
		cc, err := grpc.NewClient(r.addr("uni_grpc"), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			cancel()
			return err
		}
		c.cancel = func() { cancel(); _ = cc.Close() }
		stream, err := cc.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/"+uniStreamService+"/Consume")
		if err != nil {
			c.cancel()
			return err
		}
		if err := stream.SendMsg(in); err != nil {
			c.cancel()
			return err
		}
		_ = stream.CloseSend()
		go func() {
			for {
				out := dynamicpb.NewMessage(outType)
				if err := stream.RecvMsg(out); err != nil {
					s, _ := status.FromError(err)
					c.setClosed(&closeInfo{Code: int(s.Code()), Reason: s.Message()})
					return
				}
				js, _ := protojson.MarshalOptions{UseProtoNames: true}.Marshal(out)
				c.addMessage(decodeOrRaw(js))
			}
		}()
	default:
		cancel()
		return fmt.Errorf("unknown uni transport %q", transport)
	}
	r.clients[st.Client] = c
	return nil
}

// readLines reads newline-delimited JSON (or SSE "data:" lines).
func (c *uniClient) readLines(body io.ReadCloser, sse bool) {
	defer func() { _ = body.Close() }()
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if sse {
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		v := decodeOrRaw([]byte(line))
		if m, ok := v.(map[string]any); ok && len(m) == 0 {
			continue // ping.
		}
		c.addMessage(v)
	}
	c.setClosed(&closeInfo{Code: 0, Reason: "stream ended"})
}

func decodeOrRaw(b []byte) any {
	v, err := DecodeJSON(b)
	if err != nil {
		return "raw:" + string(b)
	}
	return v
}
