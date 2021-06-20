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
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	transport := newEventsourceTransport(r)

	c, closeFn, err := centrifuge.NewClient(r.Context(), h.node, transport)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error create client", map[string]interface{}{"error": err.Error()}))
		return
	}
	defer func() { _ = closeFn() }()
	defer close(transport.closedCh) // need to execute this after client closeFn.

	flusher := w.(http.Flusher)
	_, err = fmt.Fprintf(w, "\r\n")
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error write", map[string]interface{}{"error": err.Error()}))
		return
	}
	flusher.Flush()

	var req *protocol.ConnectRequest
	connectRequestString := r.URL.Query().Get("connect")
	if r.Method == http.MethodGet && connectRequestString != "" {
		var err error
		req, err = protocol.NewJSONParamsDecoder().DecodeConnect([]byte(connectRequestString))
		if err != nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "malformed connect request", map[string]interface{}{"error": err.Error()}))
			return
		}
	} else {
		req = &protocol.ConnectRequest{}
	}

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

	if err = c.Connect(centrifuge.ConnectRequest{}); err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error connect client", map[string]interface{}{"error": err.Error()}))
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
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error write", map[string]interface{}{"error": err.Error()}))
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
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error write", map[string]interface{}{"error": err.Error()}))
				return
			}
			flusher.Flush()
		}
	}
}
