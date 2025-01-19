package app

import (
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func logStartWarnings(cfg config.Config, cfgMeta config.Meta) {
	if cfg.Client.Insecure {
		log.Warn().Msg("INSECURE client mode enabled, make sure you understand risks")
	}
	if cfg.HttpAPI.Insecure {
		log.Warn().Msg("INSECURE HTTP API mode enabled, make sure you understand risks")
	}
	if cfg.Admin.Insecure {
		log.Warn().Msg("INSECURE admin mode enabled, make sure you understand risks")
	}
	if cfg.Debug.Enabled {
		log.Warn().Msg("DEBUG mode enabled, see on " + cfg.Debug.HandlerPrefix)
	}

	for _, key := range cfgMeta.UnknownKeys {
		log.Warn().Str("key", key).Msg("unknown key in configuration file")
	}
	for _, key := range cfgMeta.UnknownEnvs {
		log.Warn().Str("var", key).Msg("unknown var in environment")
	}
}

type httpErrorLogWriter struct {
	zerolog.Logger
}

func (w *httpErrorLogWriter) Write(data []byte) (int, error) {
	w.Logger.Warn().Msg(strings.TrimSpace(string(data)))
	return len(data), nil
}
