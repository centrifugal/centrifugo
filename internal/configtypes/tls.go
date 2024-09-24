package configtypes

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
)

// TLSConfig is a common configuration for TLS.
// It allows to configure TLS settings using different sources. The order sources are used is the following:
// 1. File to PEM
// 2. Base64 encoded PEM
// 3. Raw PEM
// It's up to the user to only use a single source of configured values. I.e. if both file and raw PEM are set
// the file will be used and raw PEM will be just ignored.
type TLSConfig struct {
	// Enabled turns on using TLS.
	Enabled bool `mapstructure:"enabled" json:"enabled" yaml:"enabled" toml:"enabled" envconfig:"enabled"`

	// CertPem is a certificate in PEM format.
	CertPem string `mapstructure:"cert_pem" json:"cert_pem" envconfig:"cert_pem" yaml:"cert_pem" toml:"cert_pem"`
	// CertPemB64 is a certificate in base64 encoded PEM format.
	CertPemB64 string `mapstructure:"cert_pem_b64" json:"cert_pem_b64" envconfig:"cert_pem_b64" yaml:"cert_pem_b64" toml:"cert_pem_b64"`
	// CertPemFile is a path to a file with certificate in PEM format.
	CertPemFile string `mapstructure:"cert_pem_file" json:"cert_pem_file" envconfig:"cert_pem_file" yaml:"cert_pem_file" toml:"cert_pem_file"`

	// KeyPem is a key in PEM format.
	KeyPem string `mapstructure:"key_pem" json:"key_pem" envconfig:"key_pem" yaml:"key_pem" toml:"key_pem"`
	// KeyPemB64 is a key in base64 encoded PEM format.
	KeyPemB64 string `mapstructure:"key_pem_b64" json:"key_pem_b64" envconfig:"key_pem_b64" yaml:"key_pem_b64" toml:"key_pem_b64"`
	// KeyPemFile is a path to a file with key in PEM format.
	KeyPemFile string `mapstructure:"key_pem_file" json:"key_pem_file" envconfig:"key_pem_file" yaml:"key_pem_file" toml:"key_pem_file"`

	// ServerCAPem is a server root CA certificate in PEM format.
	// The client uses this certificate to verify the server's certificate during the TLS handshake.
	ServerCAPem string `mapstructure:"server_ca_pem" json:"server_ca_pem" envconfig:"server_ca_pem" yaml:"server_ca_pem" toml:"server_ca_pem"`
	// ServerCAPemB64 is a server root CA certificate in base64 encoded PEM format.
	ServerCAPemB64 string `mapstructure:"server_ca_pem_b64" json:"server_ca_pem_b64" envconfig:"server_ca_pem_b64" yaml:"server_ca_pem_b64" toml:"server_ca_pem_b64"`
	// ServerCAPemFile is a path to a file with server root CA certificate in PEM format.
	ServerCAPemFile string `mapstructure:"server_ca_pem_file" json:"server_ca_pem_file" envconfig:"server_ca_pem_file" yaml:"server_ca_pem_file" toml:"server_ca_pem_file"`

	// ClientCAPem is a client CA certificate in PEM format.
	// The server uses this certificate to verify the client's certificate during the TLS handshake.
	ClientCAPem string `mapstructure:"client_ca_pem" json:"client_ca_pem" envconfig:"client_ca_pem" yaml:"client_ca_pem" toml:"client_ca_pem"`
	// ClientCAPemB64 is a client CA certificate in base64 encoded PEM format.
	ClientCAPemB64 string `mapstructure:"client_ca_pem_b64" json:"client_ca_pem_b64" envconfig:"client_ca_pem_b64" yaml:"client_ca_pem_b64" toml:"client_ca_pem_b64"`
	// ClientCAPemFile is a path to a file with client CA certificate in PEM format.
	ClientCAPemFile string `mapstructure:"client_ca_pem_file" json:"client_ca_pem_file" envconfig:"client_ca_pem_file" yaml:"client_ca_pem_file" toml:"client_ca_pem_file"`

	// InsecureSkipVerify turns off server certificate verification.
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify" json:"insecure_skip_verify" envconfig:"insecure_skip_verify" yaml:"insecure_skip_verify" toml:"insecure_skip_verify"`
	// ServerName is used to verify the hostname on the returned certificates.
	ServerName string `mapstructure:"server_name" json:"server_name" envconfig:"server_name" yaml:"server_name" toml:"server_name"`
}

func (c TLSConfig) ToGoTLSConfig() (*tls.Config, error) {
	if !c.Enabled {
		return nil, nil
	}
	return makeTLSConfig(c, os.ReadFile)
}

// ReadFileFunc is an abstraction for os.ReadFile but also io/fs.ReadFile
// wrapped with an io/fs.FS instance.
//
// Note that os.DirFS has slightly different semantics compared to the native
// filesystem APIs, see https://go.dev/issue/44279
type ReadFileFunc func(name string) ([]byte, error)

// makeTLSConfig constructs a tls.Config instance using the given configuration.
func makeTLSConfig(cfg TLSConfig, readFile ReadFileFunc) (*tls.Config, error) {
	tlsConfig := &tls.Config{}

	if cfg.CertPemFile != "" && cfg.KeyPemFile != "" {
		certPEMBlock, err := readFile(cfg.CertPemFile)
		if err != nil {
			return nil, fmt.Errorf("read TLS certificate for %s: %w", cfg.CertPemFile, err)
		}
		keyPEMBlock, err := readFile(cfg.KeyPemFile)
		if err != nil {
			return nil, fmt.Errorf("read TLS key for %s: %w", cfg.KeyPemFile, err)
		}
		cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
		if err != nil {
			return nil, fmt.Errorf("parse certificate/key pair for %s/%s: %w", cfg.CertPemFile, cfg.KeyPemFile, err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	} else if cfg.CertPemB64 != "" && cfg.KeyPemB64 != "" {
		certPem, err := base64.StdEncoding.DecodeString(cfg.CertPemB64)
		if err != nil {
			return nil, fmt.Errorf("error base64 decode certificate PEM: %w", err)
		}
		keyPem, err := base64.StdEncoding.DecodeString(cfg.KeyPemB64)
		if err != nil {
			return nil, fmt.Errorf("error base64 decode key PEM: %w", err)
		}
		cert, err := tls.X509KeyPair(certPem, keyPem)
		if err != nil {
			return nil, fmt.Errorf("error parse certificate/key pair: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	} else if cfg.CertPem != "" && cfg.KeyPem != "" {
		cert, err := tls.X509KeyPair([]byte(cfg.CertPem), []byte(cfg.KeyPem))
		if err != nil {
			return nil, fmt.Errorf("error parse certificate/key pair: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if cfg.ServerCAPemFile != "" {
		caCert, err := readFile(cfg.ServerCAPemFile)
		if err != nil {
			return nil, fmt.Errorf("read the root CA certificate for %s: %w", cfg.ServerCAPemFile, err)
		}
		caCertPool, err := newCertPoolFromPEM(caCert)
		if err != nil {
			return nil, fmt.Errorf("error parse root CA certificate: %w", err)
		}
		tlsConfig.RootCAs = caCertPool
	} else if cfg.ServerCAPemB64 != "" {
		caCert, err := base64.StdEncoding.DecodeString(cfg.ServerCAPemB64)
		if err != nil {
			return nil, fmt.Errorf("error base64 decode root CA PEM: %w", err)
		}
		caCertPool, err := newCertPoolFromPEM(caCert)
		if err != nil {
			return nil, fmt.Errorf("error parse root CA certificate: %w", err)
		}
		tlsConfig.RootCAs = caCertPool
	} else if cfg.ServerCAPem != "" {
		caCertPool, err := newCertPoolFromPEM([]byte(cfg.ServerCAPem))
		if err != nil {
			return nil, fmt.Errorf("error parse root CA certificate: %w", err)
		}
		tlsConfig.RootCAs = caCertPool
	}

	if cfg.ClientCAPemFile != "" {
		caCert, err := readFile(cfg.ClientCAPemFile)
		if err != nil {
			return nil, fmt.Errorf("read the client CA certificate for %s: %w", cfg.ClientCAPemFile, err)
		}
		caCertPool, err := newCertPoolFromPEM(caCert)
		if err != nil {
			return nil, fmt.Errorf("error parse client CA certificate: %w", err)
		}
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	} else if cfg.ClientCAPemB64 != "" {
		caCert, err := base64.StdEncoding.DecodeString(cfg.ClientCAPemB64)
		if err != nil {
			return nil, fmt.Errorf("error base64 decode client CA PEM: %w", err)
		}
		caCertPool, err := newCertPoolFromPEM(caCert)
		if err != nil {
			return nil, fmt.Errorf("error parse client CA certificate: %w", err)
		}
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	} else if cfg.ClientCAPem != "" {
		caCertPool, err := newCertPoolFromPEM([]byte(cfg.ClientCAPem))
		if err != nil {
			return nil, fmt.Errorf("error parse client CA certificate: %w", err)
		}
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	tlsConfig.ServerName = cfg.ServerName
	tlsConfig.InsecureSkipVerify = cfg.InsecureSkipVerify

	return tlsConfig, nil
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
