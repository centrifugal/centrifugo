package tools

import (
	"crypto/tls"
	"strconv"
	"testing"
	"testing/fstest"
)

const testCertPEM = `-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----`

const testKeyPEM = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIIrYSSNQFaA2Hwf1duRSxKtLYX5CB04fSeQ6tF1aY/PuoAoGCCqGSM49
AwEHoUQDQgAEPR3tU2Fta9ktY+6P9G0cWO+0kETA6SFs38GecTyudlHz6xvCdz8q 
EKTcWGekdmdDPsHloRNtsiCa697B2O9IFA==
-----END EC PRIVATE KEY-----`

var testCertificate, _ = tls.X509KeyPair([]byte(testCertPEM), []byte(testKeyPEM))

var testCertPool, _ = newCertPoolFromPEM([]byte(testCertPEM))

type testConfigGetter map[string]string

func (c testConfigGetter) GetBool(name string) bool {
	v, _ := strconv.ParseBool(c[name])
	return v
}

func (c testConfigGetter) GetString(name string) string {
	return c[name]
}

func TestMakeTLSConfig(t *testing.T) {
	testCases := []struct {
		name   string
		config testConfigGetter
		prefix string
		fsys   fstest.MapFS
		expect tls.Config
		errOK  bool
	}{{
		name: "empty",
	}, {
		name: "serverName",
		config: testConfigGetter{
			"tls_server_name": "example.com",
		},
		expect: tls.Config{
			ServerName: "example.com",
		},
	}, {
		name: "insecureSkipVerify",
		config: testConfigGetter{
			"tls_insecure_skip_verify": "true",
		},
		expect: tls.Config{
			InsecureSkipVerify: true,
		},
	}, {
		name: "certPEM",
		config: testConfigGetter{
			"tls_key_pem":  testKeyPEM,
			"tls_cert_pem": testCertPEM,
		},
		expect: tls.Config{
			Certificates: []tls.Certificate{testCertificate},
		},
	}, {
		name: "certPEMWithoutKey",
		config: testConfigGetter{
			"tls_cert_pem": testCertPEM,
		},
	}, {
		name: "keyPEMWithoutCert",
		config: testConfigGetter{
			"tls_key_pem": testKeyPEM,
		},
	}, {
		name: "badKeyPairPEM",
		config: testConfigGetter{
			"tls_key_pem":  "garbage",
			"tls_cert_pem": "garbage",
		},
		errOK: true,
	}, {
		name: "certFile",
		config: testConfigGetter{
			"tls_key":  "tls/centrifugo.key",
			"tls_cert": "tls/centrifugo.cert",
		},
		fsys: fstest.MapFS{
			"tls/centrifugo.key": &fstest.MapFile{
				Data: []byte(testKeyPEM),
			},
			"tls/centrifugo.cert": &fstest.MapFile{
				Data: []byte(testCertPEM),
			},
		},
		expect: tls.Config{
			Certificates: []tls.Certificate{testCertificate},
		},
	}, {
		name: "certFileWithoutKey",
		config: testConfigGetter{
			"tls_cert": "tls/centrifugo.cert",
		},
	}, {
		name: "keyFileWithoutCert",
		config: testConfigGetter{
			"tls_key": "tls/centrifugo.key",
		},
	}, {
		name: "missingCertFile",
		config: testConfigGetter{
			"tls_key":  "tls/centrifugo.key",
			"tls_cert": "tls/centrifugo.cert",
		},
		fsys: fstest.MapFS{
			"tls/centrifugo.key": &fstest.MapFile{
				Data: []byte(testKeyPEM),
			},
		},
		errOK: true,
	}, {
		name: "missingKeyFile",
		config: testConfigGetter{
			"tls_key":  "tls/centrifugo.key",
			"tls_cert": "tls/centrifugo.cert",
		},
		fsys: fstest.MapFS{
			"tls/centrifugo.cert": &fstest.MapFile{
				Data: []byte(testCertPEM),
			},
		},
		errOK: true,
	}, {
		name: "badKeyPairFile",
		config: testConfigGetter{
			"tls_key":  "tls/centrifugo.key",
			"tls_cert": "tls/centrifugo.cert",
		},
		fsys: fstest.MapFS{
			"tls/centrifugo.key":  &fstest.MapFile{},
			"tls/centrifugo.cert": &fstest.MapFile{},
		},
		errOK: true,
	}, {
		name: "rootCAPEM",
		config: testConfigGetter{
			"tls_root_ca_pem": testCertPEM,
		},
		expect: tls.Config{
			RootCAs: testCertPool,
		},
	}, {
		name: "badRootCAPEM",
		config: testConfigGetter{
			"tls_root_ca_pem": "garbage",
		},
		errOK: true,
	}, {
		name: "rootCAFile",
		config: testConfigGetter{
			"tls_root_ca": "certs.pem",
		},
		fsys: fstest.MapFS{
			"certs.pem": &fstest.MapFile{
				Data: []byte(testCertPEM),
			},
		},
		expect: tls.Config{
			RootCAs: testCertPool,
		},
	}, {
		name: "missingRootCAFile",
		config: testConfigGetter{
			"tls_root_ca": "certs.pem",
		},
		errOK: true,
	}, {
		name: "badRootCAFile",
		config: testConfigGetter{
			"tls_root_ca": "certs.pem",
		},
		fsys: fstest.MapFS{
			"certs.pem": &fstest.MapFile{},
		},
		errOK: true,
	}, {
		name: "prefixedWithAllFields",
		config: testConfigGetter{
			"test_tls_cert":                 "tls/centrifugo.cert",
			"test_tls_key":                  "tls/centrifugo.key",
			"test_tls_cert_pem":             "garbage",
			"test_tls_key_pem":              "garbage",
			"test_tls_root_ca":              "root.pem",
			"test_tls_root_ca_pem":          "garbage",
			"test_tls_server_name":          "example.com",
			"test_tls_insecure_skip_verify": "true",
		},
		prefix: "test_",
		fsys: fstest.MapFS{
			"tls/centrifugo.key": &fstest.MapFile{
				Data: []byte(testKeyPEM),
			},
			"tls/centrifugo.cert": &fstest.MapFile{
				Data: []byte(testCertPEM),
			},
			"root.pem": &fstest.MapFile{
				Data: []byte(testCertPEM),
			},
		},
		expect: tls.Config{
			ServerName:         "example.com",
			InsecureSkipVerify: true,
			RootCAs:            testCertPool,
			Certificates:       []tls.Certificate{testCertificate},
		},
	}, {
		name: "prefixedWithAllPEMFields",
		config: testConfigGetter{
			"test_tls_cert_pem":             testCertPEM,
			"test_tls_key_pem":              testKeyPEM,
			"test_tls_root_ca_pem":          testCertPEM,
			"test_tls_server_name":          "example.com",
			"test_tls_insecure_skip_verify": "true",
		},
		prefix: "test_",
		expect: tls.Config{
			ServerName:         "example.com",
			InsecureSkipVerify: true,
			RootCAs:            testCertPool,
			Certificates:       []tls.Certificate{testCertificate},
		},
	}}
	for i := range testCases {
		tc := &testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			config, err := MakeTLSConfig(tc.config, tc.prefix, tc.fsys.ReadFile)
			switch {
			case tc.errOK:
				if err == nil {
					t.Fatal("expected error")
				}
			case err != nil:
				t.Fatal(err)
			default:
				checkTLSConfig(t, &tc.expect, config)
			}
		})
	}
}

func checkTLSConfig(t *testing.T, a, b *tls.Config) {
	if a.ServerName != b.ServerName {
		t.Errorf(
			"expected tls.Config.ServerName to be %q, but got %q",
			a.ServerName, b.ServerName,
		)
	}
	if a.InsecureSkipVerify != b.InsecureSkipVerify {
		t.Errorf(
			"expected tls.Config.InsecureSkipVerify to be %t, but got %t",
			a.InsecureSkipVerify, b.InsecureSkipVerify,
		)
	}
	if len(a.Certificates) != len(b.Certificates) {
		// TODO: check that tls.Certificate instances are equal.
		t.Errorf(
			"expected tls.Config.Certificates length to be %d, but got %d",
			len(a.Certificates), len(b.Certificates),
		)
	}
	if !a.RootCAs.Equal(b.RootCAs) {
		t.Error("expected tls.Config.RootCAs to be equal")
	}
}
