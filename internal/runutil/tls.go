package runutil

import (
	"crypto/tls"
	stdlog "log"
	"net/http"
	"sync"

	"github.com/centrifugal/centrifugo/v5/internal/config"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

var startHTTPChallengeServerOnce sync.Once

func GetTLSConfig(cfg config.Config) (*tls.Config, error) {
	tlsEnabled := cfg.TLS.Enabled
	tlsAutocertEnabled := cfg.TLSAutocert.Enabled
	tlsAutocertHostWhitelist := cfg.TLSAutocert.HostWhitelist
	tlsAutocertCacheDir := cfg.TLSAutocert.CacheDir
	tlsAutocertEmail := cfg.TLSAutocert.Email
	tlsAutocertServerName := cfg.TLSAutocert.ServerName
	tlsAutocertHTTP := cfg.TLSAutocert.HTTP
	tlsAutocertHTTPAddr := cfg.TLSAutocert.HTTPAddr

	if tlsAutocertEnabled {
		certManager := autocert.Manager{
			Prompt: autocert.AcceptTOS,
			Email:  tlsAutocertEmail,
		}
		if tlsAutocertHostWhitelist != nil {
			certManager.HostPolicy = autocert.HostWhitelist(tlsAutocertHostWhitelist...)
		}
		if tlsAutocertCacheDir != "" {
			certManager.Cache = autocert.DirCache(tlsAutocertCacheDir)
		}

		if tlsAutocertHTTP {
			startHTTPChallengeServerOnce.Do(func() {
				// getTLSConfig can be called several times.
				acmeHTTPServer := &http.Server{
					Handler:  certManager.HTTPHandler(nil),
					Addr:     tlsAutocertHTTPAddr,
					ErrorLog: stdlog.New(&httpErrorLogWriter{log.Logger}, "", 0),
				}
				go func() {
					log.Info().Msgf("serving ACME http_01 challenge on %s", tlsAutocertHTTPAddr)
					if err := acmeHTTPServer.ListenAndServe(); err != nil {
						log.Fatal().Msgf("can't create server on %s to serve acme http challenge: %v", tlsAutocertHTTPAddr, err)
					}
				}()
			})
		}

		return &tls.Config{
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				// See https://github.com/centrifugal/centrifugo/issues/144#issuecomment-279393819
				if tlsAutocertServerName != "" && hello.ServerName == "" {
					hello.ServerName = tlsAutocertServerName
				}
				return certManager.GetCertificate(hello)
			},
			NextProtos: []string{
				"h2", "http/1.1", acme.ALPNProto,
			},
		}, nil

	} else if tlsEnabled {
		// Autocert disabled - just try to use provided SSL cert and key files.
		return cfg.TLS.ToGoTLSConfig()
	}

	return nil, nil
}