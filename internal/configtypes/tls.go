package configtypes

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"github.com/rs/zerolog"

	"github.com/rs/zerolog/log"
)

// TLSConfig is a common configuration for TLS.
// It allows to configure TLS settings using different sources. The order sources are used is the following:
// 1. File to PEM
// 2. Base64 encoded PEM
// 3. Raw PEM
// It's up to the user to only use a single source of configured values. I.e. if both file and raw PEM are set
// the file will be used and raw PEM will be just ignored. For certificate and key it's required to use the same
// source type - whether set both from file, both from base64 or both from raw string.
type TLSConfig struct {
	// Enabled turns on using TLS.
	Enabled bool `mapstructure:"enabled" json:"enabled" yaml:"enabled" toml:"enabled" envconfig:"enabled"`

	// CertPemFile is a path to a file with certificate in PEM format.
	CertPemFile string `mapstructure:"cert_pem_file" json:"cert_pem_file" envconfig:"cert_pem_file" yaml:"cert_pem_file" toml:"cert_pem_file"`
	// KeyPemFile is a path to a file with key in PEM format.
	KeyPemFile string `mapstructure:"key_pem_file" json:"key_pem_file" envconfig:"key_pem_file" yaml:"key_pem_file" toml:"key_pem_file"`

	// CertPemB64 is a certificate in base64 encoded PEM format.
	CertPemB64 string `mapstructure:"cert_pem_b64" json:"cert_pem_b64" envconfig:"cert_pem_b64" yaml:"cert_pem_b64" toml:"cert_pem_b64"`
	// KeyPemB64 is a key in base64 encoded PEM format.
	KeyPemB64 string `mapstructure:"key_pem_b64" json:"key_pem_b64" envconfig:"key_pem_b64" yaml:"key_pem_b64" toml:"key_pem_b64"`

	// CertPem is a certificate in PEM format.
	CertPem string `mapstructure:"cert_pem" json:"cert_pem" envconfig:"cert_pem" yaml:"cert_pem" toml:"cert_pem"`
	// KeyPem is a key in PEM format.
	KeyPem string `mapstructure:"key_pem" json:"key_pem" envconfig:"key_pem" yaml:"key_pem" toml:"key_pem"`

	// ServerCAPemFile is a path to a file with server root CA certificate in PEM format.
	// The client uses this certificate to verify the server's certificate during the TLS handshake.
	ServerCAPemFile string `mapstructure:"server_ca_pem_file" json:"server_ca_pem_file" envconfig:"server_ca_pem_file" yaml:"server_ca_pem_file" toml:"server_ca_pem_file"`
	// ServerCAPemB64 is a server root CA certificate in base64 encoded PEM format.
	ServerCAPemB64 string `mapstructure:"server_ca_pem_b64" json:"server_ca_pem_b64" envconfig:"server_ca_pem_b64" yaml:"server_ca_pem_b64" toml:"server_ca_pem_b64"`
	// ServerCAPem is a server root CA certificate in PEM format.
	ServerCAPem string `mapstructure:"server_ca_pem" json:"server_ca_pem" envconfig:"server_ca_pem" yaml:"server_ca_pem" toml:"server_ca_pem"`

	// ClientCAPemFile is a path to a file with client CA certificate in PEM format.
	// The server uses this certificate to verify the client's certificate during the TLS handshake.
	ClientCAPemFile string `mapstructure:"client_ca_pem_file" json:"client_ca_pem_file" envconfig:"client_ca_pem_file" yaml:"client_ca_pem_file" toml:"client_ca_pem_file"`
	// ClientCAPemB64 is a client CA certificate in base64 encoded PEM format.
	ClientCAPemB64 string `mapstructure:"client_ca_pem_b64" json:"client_ca_pem_b64" envconfig:"client_ca_pem_b64" yaml:"client_ca_pem_b64" toml:"client_ca_pem_b64"`
	// ClientCAPem is a client CA certificate in PEM format.
	ClientCAPem string `mapstructure:"client_ca_pem" json:"client_ca_pem" envconfig:"client_ca_pem" yaml:"client_ca_pem" toml:"client_ca_pem"`

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
	logger.Trace().Msg("TLS enabled")
	return makeTLSConfig(c, logger, os.ReadFile)
}

// ReadFileFunc is an abstraction for os.ReadFile but also io/fs.ReadFile
// wrapped with an io/fs.FS instance.
//
// Note that os.DirFS has slightly different semantics compared to the native
// filesystem APIs, see https://go.dev/issue/44279
type ReadFileFunc func(name string) ([]byte, error)

// makeTLSConfig constructs a tls.Config instance using the given configuration.
func makeTLSConfig(cfg TLSConfig, logger zerolog.Logger, readFile ReadFileFunc) (*tls.Config, error) {
	tlsConfig := &tls.Config{}
	if err := loadCertificate(cfg, logger, tlsConfig, readFile); err != nil {
		return nil, fmt.Errorf("error load certificate: %w", err)
	}
	if err := loadServerCA(cfg, logger, tlsConfig, readFile); err != nil {
		return nil, fmt.Errorf("error load server CA: %w", err)
	}
	if err := loadClientCA(cfg, logger, tlsConfig, readFile); err != nil {
		return nil, fmt.Errorf("error load client CA: %w", err)
	}
	tlsConfig.ServerName = cfg.ServerName
	tlsConfig.InsecureSkipVerify = cfg.InsecureSkipVerify
	logger.Trace().Str("server_name", cfg.ServerName).Bool("insecure_skip_verify", cfg.InsecureSkipVerify).Msg("TLS config options set")
	logger.Trace().Msg("TLS config created")
	return tlsConfig, nil
}

// loadCertificate loads the TLS certificate from various sources.
func loadCertificate(cfg TLSConfig, logger zerolog.Logger, tlsConfig *tls.Config, readFile ReadFileFunc) error {
	var certPEMBlock, keyPEMBlock []byte
	var err error

	switch {
	case cfg.CertPemFile != "" && cfg.KeyPemFile != "":
		logger.Trace().Str("cert_pem_file", cfg.CertPemFile).Str("key_pem_file", cfg.KeyPemFile).Msg("load TLS certificate and key from files")
		certPEMBlock, err = readFile(cfg.CertPemFile)
		if err != nil {
			return fmt.Errorf("read TLS certificate for %s: %w", cfg.CertPemFile, err)
		}
		keyPEMBlock, err = readFile(cfg.KeyPemFile)
		if err != nil {
			return fmt.Errorf("read TLS key for %s: %w", cfg.KeyPemFile, err)
		}
	case cfg.CertPemB64 != "" && cfg.KeyPemB64 != "":
		logger.Trace().Msg("load TLS certificate and key from base64 encoded strings")
		certPEMBlock, err = base64.StdEncoding.DecodeString(cfg.CertPemB64)
		if err != nil {
			return fmt.Errorf("error base64 decode certificate PEM: %w", err)
		}
		keyPEMBlock, err = base64.StdEncoding.DecodeString(cfg.KeyPemB64)
		if err != nil {
			return fmt.Errorf("error base64 decode key PEM: %w", err)
		}
	case cfg.CertPem != "" && cfg.KeyPem != "":
		logger.Trace().Msg("load TLS certificate and key from raw strings")
		certPEMBlock, keyPEMBlock = []byte(cfg.CertPem), []byte(cfg.KeyPem)
	default:
	}

	if len(certPEMBlock) > 0 && len(keyPEMBlock) > 0 {
		logger.Trace().Msg("create x509 key pair")
		cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
		if err != nil {
			return fmt.Errorf("error create x509 key pair: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	} else {
		logger.Trace().Msg("no cert or key provided, skip loading x509 key pair")
	}
	return nil
}

// loadServerCA loads the root CA from various sources.
func loadServerCA(cfg TLSConfig, logger zerolog.Logger, tlsConfig *tls.Config, readFile ReadFileFunc) error {
	caCert, err := loadPEMBlock(cfg.ServerCAPemFile, cfg.ServerCAPemB64, cfg.ServerCAPem, logger, "server CA", readFile)
	if err != nil {
		return fmt.Errorf("error load server CA certificate: %w", err)
	}
	if len(caCert) > 0 {
		logger.Trace().Msg("load server CA certificate")
		caCertPool, err := newCertPoolFromPEM(caCert)
		if err != nil {
			return fmt.Errorf("error create server CA certificate pool: %w", err)
		}
		tlsConfig.RootCAs = caCertPool
	} else {
		logger.Trace().Msg("no server CA certificate provided")
	}
	return nil
}

// loadClientCA loads the client CA from various sources.
func loadClientCA(cfg TLSConfig, logger zerolog.Logger, tlsConfig *tls.Config, readFile ReadFileFunc) error {
	caCert, err := loadPEMBlock(cfg.ClientCAPemFile, cfg.ClientCAPemB64, cfg.ClientCAPem, logger, "client CA", readFile)
	if err != nil {
		return err
	}
	if len(caCert) > 0 {
		logger.Trace().Msg("load client CA certificate")
		caCertPool, err := newCertPoolFromPEM(caCert)
		if err != nil {
			return fmt.Errorf("error create client CA certificate pool: %w", err)
		}
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	} else {
		logger.Trace().Msg("no client CA certificate provided")
	}
	return nil
}

// loadPEMBlock attempts to load PEM data from a file, base64 string, or raw string.
func loadPEMBlock(file, b64, raw string, logger zerolog.Logger, certType string, readFile ReadFileFunc) ([]byte, error) {
	var pemBlock []byte
	var err error
	if file != "" {
		logger.Trace().Str("file", file).Msg("load PEM block of " + certType + " from file")
		pemBlock, err = readFile(file)
		if err != nil {
			return nil, fmt.Errorf("read PEM block for %s: %w", file, err)
		}
	} else if b64 != "" {
		logger.Trace().Msg("load PEM block of " + certType + " from base64 encoded string")
		pemBlock, err = base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("error base64 decode PEM block: %w", err)
		}
	} else if raw != "" {
		logger.Trace().Msg("load PEM block of " + certType + " from raw string")
		pemBlock = []byte(raw)
	}
	return pemBlock, nil
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
