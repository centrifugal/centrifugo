package unihttpstream

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/centrifugo/v5/internal/configtypes"
	"github.com/centrifugal/protocol"
)

type Handler struct {
	node     *centrifuge.Node
	config   Config
	pingPong centrifuge.PingPongConfig
}

func NewHandler(n *centrifuge.Node, c Config, pingPong centrifuge.PingPongConfig) *Handler {
	return &Handler{
		node:     n,
		config:   c,
		pingPong: pingPong,
	}
}

const streamWriteTimeout = time.Second

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	var req *protocol.ConnectRequest
	if r.Method == http.MethodPost {
		maxBytesSize := int64(h.config.MaxRequestBodySize)
		r.Body = http.MaxBytesReader(w, r.Body, maxBytesSize)
		connectRequestData, err := io.ReadAll(r.Body)
		if err != nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "error reading uni http stream request body", map[string]any{"error": err.Error()}))
			if len(connectRequestData) >= int(maxBytesSize) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			return
		}
		err = json.Unmarshal(connectRequestData, &req)
		if err != nil {
			if h.node.LogEnabled(centrifuge.LogLevelDebug) {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "malformed connect request", map[string]any{"error": err.Error()}))
			}
			return
		}
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	transport := newStreamTransport(r, h.pingPong)
	c, closeFn, err := centrifuge.NewClient(r.Context(), h.node, transport)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error create client", map[string]any{"error": err.Error(), "transport": "uni_http_stream"}))
		return
	}
	defer func() { _ = closeFn() }()
	defer close(transport.closedCh) // need to execute this after client closeFn.

	if h.node.LogEnabled(centrifuge.LogLevelDebug) {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection established", map[string]any{"transport": transport.Name(), "client": c.ID()}))
		defer func(started time.Time) {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection completed", map[string]any{"duration": time.Since(started).String(), "transport": transport.Name(), "client": c.ID()}))
		}(time.Now())
	}

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

	if h.config.ConnectCodeToHTTPStatus.Enabled {
		err = c.ConnectNoErrorToDisconnect(connectRequest)
		if err != nil {
			resp, ok := configtypes.ConnectErrorToToHTTPResponse(err, h.config.ConnectCodeToHTTPStatus.Transforms)
			if ok {
				w.WriteHeader(resp.Status)
				_, _ = w.Write([]byte(resp.Body))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(http.StatusText(http.StatusInternalServerError)))
			return
		}
	} else {
		c.Connect(connectRequest)
	}

	if r.ProtoMajor == 1 {
		// An endpoint MUST NOT generate an HTTP/2 message containing connection-specific header fields.
		// Source: RFC7540.
		w.Header().Set("Connection", "keep-alive")
	}
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Cache-Control", "private, no-cache, no-store, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expire", "0")
	w.WriteHeader(http.StatusOK)

	_, ok := w.(http.Flusher)
	if !ok {
		return
	}

	rc := http.NewResponseController(w)

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
			_ = rc.SetWriteDeadline(time.Now().Add(streamWriteTimeout))
			_, err = w.Write(data)
			if err != nil {
				return
			}
			_, err = w.Write([]byte("\n"))
			if err != nil {
				return
			}
			_ = rc.Flush()
		}
	}
}
