package app

import (
	"crypto/tls"
	stdlog "log"
	"net/http"
	"sync"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

var startHTTPChallengeServerOnce sync.Once

func GetTLSConfig(cfg config.Config) (*tls.Config, error) {
	tlsEnabled := cfg.HTTP.TLS.Enabled
	tlsAutocertEnabled := cfg.HTTP.TLSAutocert.Enabled
	tlsAutocertHostWhitelist := cfg.HTTP.TLSAutocert.HostWhitelist
	tlsAutocertCacheDir := cfg.HTTP.TLSAutocert.CacheDir
	tlsAutocertEmail := cfg.HTTP.TLSAutocert.Email
	tlsAutocertServerName := cfg.HTTP.TLSAutocert.ServerName
	tlsAutocertHTTP := cfg.HTTP.TLSAutocert.HTTP
	tlsAutocertHTTPAddr := cfg.HTTP.TLSAutocert.HTTPAddr

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
						log.Fatal().Err(err).Msgf("can't create server on %s to serve acme http challenge", tlsAutocertHTTPAddr)
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
		return cfg.HTTP.TLS.ToGoTLSConfig("http_server")
	}

	return nil, nil
}
