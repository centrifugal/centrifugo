package unisse

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
)

type Handler struct {
	node   *centrifuge.Node
	config Config
}

func NewHandler(n *centrifuge.Node, c Config) *Handler {
	return &Handler{
		node:   n,
		config: c,
	}
}

// Since SSE is a GET request we are looking for connect request in URL params.
// This should be a properly encoded JSON object.
const connectUrlParam = "cf_connect"

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req *protocol.ConnectRequest
	if r.Method == http.MethodGet {
		connectRequestString := r.URL.Query().Get(connectUrlParam)
		if connectRequestString != "" {
			var err error
			err = json.Unmarshal([]byte(connectRequestString), &req)
			if err != nil {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "malformed connect request", map[string]interface{}{"error": err.Error()}))
				return
			}
		} else {
			req = &protocol.ConnectRequest{}
		}
	} else if r.Method == http.MethodPost {
		maxBytesSize := int64(h.config.MaxRequestBodySize)
		r.Body = http.MaxBytesReader(w, r.Body, maxBytesSize)
		connectRequestData, err := io.ReadAll(r.Body)
		if err != nil {
			if len(connectRequestData) >= int(maxBytesSize) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error reading body", map[string]interface{}{"error": err.Error()}))
			return
		}
		err = json.Unmarshal(connectRequestData, &req)
		if err != nil {
			if h.node.LogEnabled(centrifuge.LogLevelDebug) {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "malformed connect request", map[string]interface{}{"error": err.Error()}))
			}
			return
		}
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	transport := newEventsourceTransport(r, h.config.PingPongConfig)
	c, closeFn, err := centrifuge.NewClient(r.Context(), h.node, transport)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error create client", map[string]interface{}{"error": err.Error(), "transport": "uni_sse"}))
		return
	}
	defer func() { _ = closeFn() }()
	defer close(transport.closedCh) // need to execute this after client closeFn.

	if h.node.LogEnabled(centrifuge.LogLevelDebug) {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection established", map[string]interface{}{"transport": transport.Name(), "client": c.ID()}))
		defer func(started time.Time) {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection completed", map[string]interface{}{"duration": time.Since(started), "transport": transport.Name(), "client": c.ID()}))
		}(time.Now())
	}

	if r.ProtoMajor == 1 {
		// An endpoint MUST NOT generate an HTTP/2 message containing connection-specific header fields.
		// Source: RFC7540.
		w.Header().Set("Connection", "keep-alive")
	}
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache, no-store, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expire", "0")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	_, err = w.Write([]byte("\r\n"))
	if err != nil {
		return
	}
	flusher.Flush()

	connectRequest := centrifuge.ConnectRequest{
		Token:   req.Token,
		Data:    req.Data,
		Name:    req.Name,
		Version: req.Version,
	}
	if req.Subs != nil {
		subs := make(map[string]centrifuge.SubscribeRequest, len(req.Subs))
		for k, v := range req.Subs {
			subs[k] = centrifuge.SubscribeRequest{
				Recover: v.Recover,
				Offset:  v.Offset,
				Epoch:   v.Epoch,
			}
		}
		connectRequest.Subs = subs
	}

	c.Connect(connectRequest)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-transport.disconnectCh:
			return
		case data, ok := <-transport.messages:
			if !ok {
				return
			}
			_, err = w.Write([]byte("data: " + string(data) + "\n\n"))
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
