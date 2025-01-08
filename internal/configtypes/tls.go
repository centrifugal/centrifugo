package configtypes

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// TLSConfig is a common configuration for TLS.
type TLSConfig struct {
	// Enabled turns on using TLS.
	Enabled bool `mapstructure:"enabled" json:"enabled" yaml:"enabled" toml:"enabled" envconfig:"enabled"`
	// CertPem is a PEM certificate.
	CertPem PEMData `mapstructure:"cert_pem" json:"cert_pem" envconfig:"cert_pem" yaml:"cert_pem" toml:"cert_pem"`
	// KeyPem is a path to a file with key in PEM format.
	KeyPem PEMData `mapstructure:"key_pem" json:"key_pem" envconfig:"key_pem" yaml:"key_pem" toml:"key_pem"`
	// ServerCAPemFile is a server root CA certificate in PEM format.
	// The client uses this certificate to verify the server's certificate during the TLS handshake.
	ServerCAPem PEMData `mapstructure:"server_ca_pem" json:"server_ca_pem" envconfig:"server_ca_pem" yaml:"server_ca_pem" toml:"server_ca_pem"`
	// ClientCAPem is a client CA certificate in PEM format.
	// The server uses this certificate to verify the client's certificate during the TLS handshake.
	ClientCAPem PEMData `mapstructure:"client_ca_pem" json:"client_ca_pem" envconfig:"client_ca_pem" yaml:"client_ca_pem" toml:"client_ca_pem"`
	// InsecureSkipVerify turns off server certificate verification.
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify" json:"insecure_skip_verify" envconfig:"insecure_skip_verify" yaml:"insecure_skip_verify" toml:"insecure_skip_verify"`
	// ServerName is used to verify the hostname on the returned certificates.
	ServerName string `mapstructure:"server_name" json:"server_name" envconfig:"server_name" yaml:"server_name" toml:"server_name"`
}

func (c TLSConfig) ToGoTLSConfig(logTraceEntity string) (*tls.Config, error) {
	if !c.Enabled {
		return nil, nil
	}
	logger := log.With().Str("entity", logTraceEntity).Logger()
	logger.Debug().Msg("TLS enabled")
	tlsConfig, err := makeTLSConfig(c, logger, os.ReadFile, os.Stat)
	if err != nil {
		return nil, fmt.Errorf("error make TLS config (for %s): %w", logTraceEntity, err)
	}
	return tlsConfig, nil
}

// ReadFileFunc is like os.ReadFile but helps in testing.
type ReadFileFunc func(name string) ([]byte, error)

// StatFileFunc is like os.Stat but helps in testing.
type StatFileFunc func(name string) (os.FileInfo, error)

// makeTLSConfig constructs a tls.Config instance using the given configuration.
func makeTLSConfig(cfg TLSConfig, logger zerolog.Logger, readFile ReadFileFunc, statFile StatFileFunc) (*tls.Config, error) {
	tlsConfig := &tls.Config{}
	if err := loadCertificate(cfg, logger, tlsConfig, readFile, statFile); err != nil {
		return nil, fmt.Errorf("error load certificate: %w", err)
	}
	if err := loadServerCA(cfg, logger, tlsConfig, readFile, statFile); err != nil {
		return nil, fmt.Errorf("error load server CA: %w", err)
	}
	if err := loadClientCA(cfg, logger, tlsConfig, readFile, statFile); err != nil {
		return nil, fmt.Errorf("error load client CA: %w", err)
	}
	tlsConfig.ServerName = cfg.ServerName
	tlsConfig.InsecureSkipVerify = cfg.InsecureSkipVerify
	logger.Debug().Str("server_name", cfg.ServerName).Bool("insecure_skip_verify", cfg.InsecureSkipVerify).Msg("TLS config options set")
	logger.Debug().Msg("TLS config created")
	return tlsConfig, nil
}

// loadCertificate loads the TLS certificate from various sources.
func loadCertificate(cfg TLSConfig, logger zerolog.Logger, tlsConfig *tls.Config, readFile ReadFileFunc, statFile StatFileFunc) error {
	var certPEMBlock, keyPEMBlock []byte
	var err error

	switch {
	case cfg.CertPem != "" && cfg.KeyPem != "":
		var pemSource string
		certPEMBlock, pemSource, err = cfg.CertPem.Load(statFile, readFile)
		if err != nil {
			return fmt.Errorf("load TLS certificate: %w", err)
		}
		logger.Debug().Str("pem_source", pemSource).Msg("loaded PEM certificate")
		keyPEMBlock, pemSource, err = cfg.KeyPem.Load(statFile, readFile)
		if err != nil {
			return fmt.Errorf("load TLS key: %w", err)
		}
		logger.Debug().Str("pem_source", pemSource).Msg("loaded PEM key")
	default:
	}

	if len(certPEMBlock) > 0 && len(keyPEMBlock) > 0 {
		logger.Debug().Msg("create x509 key pair")
		cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
		if err != nil {
			return fmt.Errorf("error create x509 key pair: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	} else {
		logger.Debug().Msg("no cert or key provided, skip loading x509 key pair")
	}
	return nil
}

// loadServerCA loads the root CA from various sources.
func loadServerCA(cfg TLSConfig, logger zerolog.Logger, tlsConfig *tls.Config, readFile ReadFileFunc, statFile StatFileFunc) error {
	if cfg.ServerCAPem == "" {
		logger.Debug().Msg("no server CA certificate provided")
		return nil
	}
	caCert, err := loadPEMData(cfg.ServerCAPem, logger, "server CA", readFile, statFile)
	if err != nil {
		return fmt.Errorf("error load server CA certificate: %w", err)
	}
	if len(caCert) > 0 {
		logger.Debug().Msg("load server CA certificate")
		caCertPool, err := newCertPoolFromPEM(caCert)
		if err != nil {
			return fmt.Errorf("error create server CA certificate pool: %w", err)
		}
		tlsConfig.RootCAs = caCertPool
	} else {
		logger.Debug().Msg("no server CA certificate provided")
	}
	return nil
}

// loadClientCA loads the client CA from various sources.
func loadClientCA(cfg TLSConfig, logger zerolog.Logger, tlsConfig *tls.Config, readFile ReadFileFunc, statFile StatFileFunc) error {
	if cfg.ClientCAPem == "" {
		logger.Debug().Msg("no client CA certificate provided")
		return nil
	}
	caCert, err := loadPEMData(cfg.ClientCAPem, logger, "client CA", readFile, statFile)
	if err != nil {
		return err
	}
	if len(caCert) > 0 {
		logger.Debug().Msg("load client CA certificate")
		caCertPool, err := newCertPoolFromPEM(caCert)
		if err != nil {
			return fmt.Errorf("error create client CA certificate pool: %w", err)
		}
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	} else {
		logger.Debug().Msg("no client CA certificate provided")
	}
	return nil
}

// loadPEMData attempts to load PEM data from a file, base64 string, or raw string.
func loadPEMData(pemData PEMData, logger zerolog.Logger, certType string, readFile ReadFileFunc, statFile StatFileFunc) ([]byte, error) {
	pemBytes, pemSource, err := pemData.Load(statFile, readFile)
	if err != nil {
		return nil, fmt.Errorf("load PEM block for %s: %w", certType, err)
	}
	logger.Debug().Str("pem_source", pemSource).Msg("loaded PEM data")
	return pemBytes, nil
}

// newCertPoolFromPEM returns certificate pool for the given PEM-encoded
// certificate bundle. Note that it currently ignores invalid blocks.
func newCertPoolFromPEM(pem []byte) (*x509.CertPool, error) {
	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(pem)
	if !ok {
		return nil, errors.New("no valid certificates found")
	}
	return certPool, nil
}

type TLSAutocert struct {
	Enabled       bool     `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HostWhitelist []string `mapstructure:"host_whitelist" json:"host_whitelist" envconfig:"host_whitelist" yaml:"host_whitelist" toml:"host_whitelist"`
	CacheDir      string   `mapstructure:"cache_dir" json:"cache_dir" envconfig:"cache_dir" yaml:"cache_dir" toml:"cache_dir"`
	Email         string   `mapstructure:"email" json:"email" envconfig:"email" yaml:"email" toml:"email"`
	ServerName    string   `mapstructure:"server_name" json:"server_name" envconfig:"server_name" yaml:"server_name" toml:"server_name"`
	HTTP          bool     `mapstructure:"http" json:"http" envconfig:"http" yaml:"http" toml:"http"`
	HTTPAddr      string   `mapstructure:"http_addr" json:"http_addr" envconfig:"http_addr" default:":80" yaml:"http_addr" toml:"http_addr"`
}
