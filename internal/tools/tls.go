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
	if v.GetString(keyPrefix+"tls_cert") != "" && v.GetString(keyPrefix+"tls_key") != "" {
		certPEMBlock, err := readFile(v.GetString(keyPrefix + "tls_cert"))
		if err != nil {
			return nil, err
		}
		keyPEMBlock, err := readFile(v.GetString(keyPrefix + "tls_key"))
		if err != nil {
			return nil, err
		}
		cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
		if err != nil {
			return nil, fmt.Errorf("could not read the certificate/key: %s", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	} else if v.GetString(keyPrefix+"tls_cert_pem") != "" && v.GetString(keyPrefix+"tls_key_pem") != "" {
		cert, err := tls.X509KeyPair([]byte(v.GetString(keyPrefix+"tls_cert_pem")), []byte(v.GetString(keyPrefix+"tls_key_pem")))
		if err != nil {
			return nil, fmt.Errorf("error creating X509 key pair: %s", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	if v.GetString(keyPrefix+"tls_root_ca") != "" {
		caCert, err := readFile(v.GetString(keyPrefix + "tls_root_ca"))
		if err != nil {
			return nil, fmt.Errorf("can not read the CA certificate: %s", err)
		}
		caCertPool := x509.NewCertPool()
		ok := caCertPool.AppendCertsFromPEM(caCert)
		if !ok {
			return nil, errors.New("can not parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	} else if v.GetString(keyPrefix+"tls_root_ca_pem") != "" {
		caCertPool := x509.NewCertPool()
		ok := caCertPool.AppendCertsFromPEM([]byte(v.GetString(keyPrefix + "tls_root_ca_pem")))
		if !ok {
			return nil, errors.New("can not parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}
	if v.GetString(keyPrefix+"tls_server_name") != "" {
		tlsConfig.ServerName = v.GetString(keyPrefix + "tls_server_name")
	}
	if v.GetBool(keyPrefix + "tls_insecure_skip_verify") {
		tlsConfig.InsecureSkipVerify = true
	}
	return tlsConfig, nil
}
