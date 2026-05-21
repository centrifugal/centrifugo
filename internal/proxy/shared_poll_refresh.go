package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// SharedPollRefreshProxy allows to send SharedPollRefresh requests.
type SharedPollRefreshProxy interface {
	ProxySharedPollRefresh(context.Context, *proxyproto.SharedPollRefreshRequest) (*proxyproto.SharedPollRefreshResponse, error)
	// Protocol for metrics and logging.
	Protocol() string
}
