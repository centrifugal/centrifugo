package unistream

import (
	"io/ioutil"
	"log"
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
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	if r.Method == http.MethodOptions {
		return
	}

	transport := newStreamTransport(r)

	c, closeFn, err := centrifuge.NewClient(r.Context(), h.node, transport)
	if err != nil {
		log.Printf("error creating client: %v", err)
		return
	}
	defer func() { _ = closeFn() }()
	defer close(transport.closedCh) // need to execute this after client closeFn.

	var req *protocol.ConnectRequest
	if r.Method == http.MethodPost {
		connectRequestData, err := ioutil.ReadAll(r.Body)
		if err != nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error reading body", map[string]interface{}{"error": err.Error()}))
			return
		}
		req, err = protocol.NewJSONParamsDecoder().DecodeConnect(connectRequestData)
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

	err = c.Connect(connectRequest)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error connect client", map[string]interface{}{"error": err.Error()}))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "ResponseWriter does not support Flusher", map[string]interface{}{}))
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
			_, err = w.Write([]byte("null\n"))
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
			_, err = w.Write(data)
			if err != nil {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error write", map[string]interface{}{"error": err.Error()}))
				return
			}
			_, err = w.Write([]byte("\n"))
			if err != nil {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error write", map[string]interface{}{"error": err.Error()}))
				return
			}
			flusher.Flush()
		}
	}
}
