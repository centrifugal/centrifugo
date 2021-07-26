package unisse

import (
	"fmt"
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

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req *protocol.ConnectRequest
	connectRequestString := r.URL.Query().Get("connect")
	if r.Method == http.MethodGet && connectRequestString != "" {
		var err error
		req, err = protocol.NewJSONParamsDecoder().DecodeConnect([]byte(connectRequestString))
		if err != nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "malformed connect request", map[string]interface{}{"error": err.Error()}))
			return
		}
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	transport := newEventsourceTransport(r)
	c, closeFn, err := centrifuge.NewClient(r.Context(), h.node, transport)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error create client", map[string]interface{}{"error": err.Error(), "transport": "uni_sse"}))
		return
	}
	defer func() { _ = closeFn() }()
	defer close(transport.closedCh) // need to execute this after client closeFn.

	h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection established", map[string]interface{}{"transport": transport.Name(), "client": c.ID()}))
	defer func(started time.Time) {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection completed", map[string]interface{}{"duration": time.Since(started), "transport": transport.Name(), "client": c.ID()}))
	}(time.Now())

	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache, no-store, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expire", "0")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	_, err = fmt.Fprintf(w, "\r\n")
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
		subs := make(map[string]centrifuge.SubscribeRequest)
		for k, v := range connectRequest.Subs {
			subs[k] = centrifuge.SubscribeRequest{
				Recover: v.Recover,
				Offset:  v.Offset,
				Epoch:   v.Epoch,
			}
		}
	}

	if err = c.Connect(connectRequest); err != nil {
		return
	}

	pingInterval := 25 * time.Second
	tick := time.NewTicker(pingInterval)
	defer tick.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-transport.disconnectCh:
			return
		case <-tick.C:
			_, err = w.Write([]byte("event: ping\ndata:\n\n"))
			if err != nil {
				return
			}
			flusher.Flush()
		case data, ok := <-transport.messages:
			if !ok {
				return
			}
			tick.Reset(pingInterval)
			_, err = w.Write([]byte("data: " + string(data) + "\n\n"))
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
