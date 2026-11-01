package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	stdlog "log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// acmeServer serves the ACME http_01 challenge.
type acmeServer struct {
	server *http.Server
	addr   string
	ln     net.Listener
}

// listenACME binds the listener of the ACME challenge server, if any.
func (a *App) listenACME() error {
	s := a.acmeServer
	if s == nil || s.ln != nil {
		return nil
	}
	addr := s.addr
	if addr == "" {
		addr = ":http" // As http.Server.ListenAndServe.
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return logFatal(err, fmt.Sprintf("can't create server on %s to serve acme http challenge", s.addr))
	}
	a.lifecycle.OnStop("ACME challenge listener", func(context.Context) error {
		_ = ln.Close()
		return nil
	})
	a.addAddr(ln.Addr())
	s.ln = ln
	return nil
}

// serveACME serves the ACME challenge in a goroutine tracked by wg.
func (a *App) serveACME(wg *sync.WaitGroup) {
	s := a.acmeServer
	if s == nil || s.ln == nil {
		return
	}
	log.Info().Msgf("serving ACME http_01 challenge on %s", s.addr)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.server.Serve(s.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.serveError(err, fmt.Sprintf("can't create server on %s to serve acme http challenge", s.addr))
		}
	}()
}

// getTLSConfig returns the TLS config of the external HTTP server.
func (a *App) getTLSConfig(cfg config.Config) (*tls.Config, error) {
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

		if tlsAutocertHTTP && a.acmeServer == nil {
			a.acmeServer = &acmeServer{
				server: &http.Server{
					Handler:           certManager.HTTPHandler(nil),
					Addr:              tlsAutocertHTTPAddr,
					ErrorLog:          stdlog.New(&httpErrorLogWriter{log.Logger}, "", 0),
					ReadHeaderTimeout: 10 * time.Second,
					ReadTimeout:       30 * time.Second,
					WriteTimeout:      10 * time.Second,
				},
				addr: tlsAutocertHTTPAddr,
			}
		}

		return &tls.Config{
			MinVersion: tls.VersionTLS12,
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
