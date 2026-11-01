package app

import (
	"net/http"
	"sync"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/origin"

	"github.com/rs/zerolog/log"
)

var warnAllowedOriginsOnce sync.Once

// getCheckOrigin returns the origin check of connection endpoints. An error
// is logged before it is returned.
func getCheckOrigin(cfg config.Config) (func(r *http.Request) bool, error) {
	allowedOrigins := cfg.Client.AllowedOrigins
	if len(allowedOrigins) == 0 {
		return func(r *http.Request) bool {
			// Only allow connections without Origin in this case.
			originHeader := r.Header.Get("Origin")
			if originHeader == "" {
				return true
			}
			log.Info().Str("origin", originHeader).Msg("request Origin is not authorized due to empty allowed_origins")
			return false
		}, nil
	}
	originChecker, err := origin.NewPatternChecker(allowedOrigins)
	if err != nil {
		return nil, logFatal(err, "error creating origin checker")
	}
	if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
		// Fast path for *.
		warnAllowedOriginsOnce.Do(func() {
			log.Warn().Msg("usage of allowed_origins * is discouraged for security reasons, consider setting exact list of origins")
		})
		return func(r *http.Request) bool {
			return true
		}, nil
	}
	return func(r *http.Request) bool {
		ok := originChecker.Check(r)
		if !ok {
			log.Info().Str("origin", r.Header.Get("Origin")).Strs("allowed_origins", allowedOrigins).Msg("request Origin is not authorized")
			return false
		}
		return true
	}, nil
}
