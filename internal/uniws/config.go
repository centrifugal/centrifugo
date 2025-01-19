package uniws

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
)

// Defaults.
const (
	DefaultWebsocketPingInterval     = 25 * time.Second
	DefaultWebsocketWriteTimeout     = 1 * time.Second
	DefaultWebsocketMessageSizeLimit = 65536 // 64KB
)

type Config = configtypes.UniWebSocket

func sameHostOriginCheck() func(r *http.Request) bool {
	return func(r *http.Request) bool {
		err := checkSameHost(r)
		return err == nil
	}
}

func checkSameHost(r *http.Request) error {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return nil
	}
	u, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("failed to parse Origin header %q: %w", origin, err)
	}
	if strings.EqualFold(r.Host, u.Host) {
		return nil
	}
	return fmt.Errorf("request Origin %q is not authorized for Host %q", origin, r.Host)
}
