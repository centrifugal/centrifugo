package tools

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
)

type ConfigGetter interface {
	GetBool(name string) bool
	GetString(name string) string
}

// ReadFileFunc is an abstraction for os.ReadFile but also io/fs.ReadFile
// wrapped with an io/fs.FS instance.
//
// Note that os.DirFS has slightly different semantics compared to the native
// filesystem APIs, see https://go.dev/issue/44279
type ReadFileFunc func(name string) ([]byte, error)

// MakeTLSConfig constructs a tls.Config instance using the given configuration
// scoped under key prefix.
func MakeTLSConfig(v ConfigGetter, keyPrefix string, readFile ReadFileFunc) (*tls.Config, error) {
	tlsConfig := &tls.Config{}

	loaders := []tlsConfigLoader{
		chainTLSConfigLoaders(loadCertFromFile, loadCertFromPEM),
		chainTLSConfigLoaders(loadRootCAFromFile, loadRootCAFromPEM),
	}
	for _, loadConfig := range loaders {
		if _, err := loadConfig(tlsConfig, v, keyPrefix, readFile); err != nil {
			return nil, err
		}
	}

	tlsConfig.ServerName = v.GetString(keyPrefix + "tls_server_name")
	tlsConfig.InsecureSkipVerify = v.GetBool(keyPrefix + "tls_insecure_skip_verify")

	return tlsConfig, nil
}

// tlsConfigLoader is a function that loads TLS from the given ConfigGetter.
// It returns false, nil if configuration does not exist, true, nil on success,
// or true, err ≠ nil if there was an error loading the configuration.
type tlsConfigLoader func(c *tls.Config, v ConfigGetter, keyPrefix string, readFile ReadFileFunc) (bool, error)

// chainTLSConfigLoaders returns tlsConfigLoader function that attempts to load
// TLS configuration until either a configuration is found or an error occurs.
func chainTLSConfigLoaders(loaders ...tlsConfigLoader) tlsConfigLoader {
	return func(c *tls.Config, v ConfigGetter, keyPrefix string, readFile ReadFileFunc) (bool, error) {
		for _, f := range loaders {
			found, err := f(c, v, keyPrefix, readFile)
			if found || err != nil {
				return found, err
			}
		}
		return false, nil
	}
}

// loadCertFromFile loads the TLS configuration with certificate from key pair
// files containing PEM-encoded TLS key and certificate.
func loadCertFromFile(tlsConfig *tls.Config, v ConfigGetter, keyPrefix string, readFile ReadFileFunc) (bool, error) {
	certFileKeyName, keyFileKeyName := keyPrefix+"tls_cert", keyPrefix+"tls_key"

	certFile, keyFile := v.GetString(certFileKeyName), v.GetString(keyFileKeyName)
	if certFile == "" || keyFile == "" {
		return false, nil
	}

	certPEMBlock, err := readFile(certFile)
	if err != nil {
		return true, fmt.Errorf("read TLS certificate for %s: %w", certFileKeyName, err)
	}
	keyPEMBlock, err := readFile(keyFile)
	if err != nil {
		return true, fmt.Errorf("read TLS key for %s: %w", keyFileKeyName, err)
	}

	cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
	if err != nil {
		return true, fmt.Errorf("parse certificate/key pair for %s/%s: %w", certFileKeyName, keyFileKeyName, err)
	}

	tlsConfig.Certificates = []tls.Certificate{cert}

	return true, nil
}

// loadCertFromPEM loads the TLS configuration with certificate from key pair
// strings containing PEM-encoded TLS key and certificate.
func loadCertFromPEM(tlsConfig *tls.Config, v ConfigGetter, keyPrefix string, _ ReadFileFunc) (bool, error) {
	certPEMKeyName, keyPEMKeyName := keyPrefix+"tls_cert_pem", keyPrefix+"tls_key_pem"

	certPEM, keyPEM := v.GetString(certPEMKeyName), v.GetString(keyPEMKeyName)
	if certPEM == "" || keyPEM == "" {
		return false, nil
	}

	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return true, fmt.Errorf("parse certificate/key pair for %s/%s: %w", certPEMKeyName, keyPEMKeyName, err)
	}

	tlsConfig.Certificates = []tls.Certificate{cert}

	return true, nil
}

// loadRootCAFromFile loads the TLS configuration with root CA bundle from file
// containing PEM-encoded certificates.
func loadRootCAFromFile(tlsConfig *tls.Config, v ConfigGetter, keyPrefix string, readFile ReadFileFunc) (bool, error) {
	keyName := keyPrefix + "tls_root_ca"

	rootCAFile := v.GetString(keyName)
	if rootCAFile == "" {
		return false, nil
	}

	caCert, err := readFile(rootCAFile)
	if err != nil {
		return true, fmt.Errorf("read the root CA certificate for %s: %w", keyName, err)
	}

	caCertPool, err := newCertPoolFromPEM(caCert)
	if err != nil {
		return true, fmt.Errorf("parse root CA certificate for %s: %w", keyName, err)
	}

	tlsConfig.RootCAs = caCertPool

	return true, nil
}

// loadRootCAFromFile loads the TLS configuration with root CA bundle from
// string containing PEM-encoded certificates.
func loadRootCAFromPEM(tlsConfig *tls.Config, v ConfigGetter, keyPrefix string, _ ReadFileFunc) (bool, error) {
	keyName := keyPrefix + "tls_root_ca_pem"

	rootCAPEM := v.GetString(keyName)
	if rootCAPEM == "" {
		return false, nil
	}

	caCertPool, err := newCertPoolFromPEM([]byte(rootCAPEM))
	if err != nil {
		return true, fmt.Errorf("parse root CA certificate for %s: %w", keyName, err)
	}

	tlsConfig.RootCAs = caCertPool

	return true, nil
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
