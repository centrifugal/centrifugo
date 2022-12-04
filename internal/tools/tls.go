package tools

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/FZambia/viper-lite"
)

func MakeTLSConfig(v *viper.Viper, keyPrefix string) (*tls.Config, error) {
	tlsConfig := &tls.Config{}
	if v.GetString(keyPrefix+"tls_cert") != "" && v.GetString(keyPrefix+"tls_key") != "" {
		cert, err := tls.LoadX509KeyPair(v.GetString(keyPrefix+"tls_cert"), v.GetString(keyPrefix+"tls_key"))
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
		caCert, err := os.ReadFile(v.GetString(keyPrefix + "tls_root_ca"))
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
	if v.GetBool(keyPrefix+"tls_skip_verify") || v.GetBool(keyPrefix+"tls_insecure_skip_verify") { // TODO v5: remove tls_skip_verify.
		tlsConfig.InsecureSkipVerify = true
	}
	return tlsConfig, nil
}
