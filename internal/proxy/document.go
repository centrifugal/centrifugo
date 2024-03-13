package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
)

// DocumentProxy allows loading documents from a source of truth.
type DocumentProxy interface {
	LoadDocuments(context.Context, *proxyproto.LoadDocumentsRequest) (*proxyproto.LoadDocumentsResponse, error)
	// Protocol for metrics and logging.
	Protocol() string
}
