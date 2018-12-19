package health

import (
	"net/http"

	"github.com/centrifugal/centrifuge"
)

// Config of health check handler.
type Config struct{}

// Handler handles health endpoint.
type Handler struct {
	mux    *http.ServeMux
	node   *centrifuge.Node
	config Config
}

// NewHandler creates new Handler.
func NewHandler(n *centrifuge.Node, c Config) *Handler {
	h := &Handler{
		node:   n,
		config: c,
	}
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{}`))
}
