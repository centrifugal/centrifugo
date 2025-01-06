package configtypes

import (
	"encoding/base64"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

func TestPEMData_Load(t *testing.T) {
	mockStatFile := func(name string) (os.FileInfo, error) {
		if name != "test-file" {
			return nil, os.ErrNotExist
		}
		return nil, nil
	}
	mockReadFile := func(name string) ([]byte, error) {
		if name != "test-file" {
			return nil, os.ErrNotExist
		}
		return []byte(testPEMCert), nil
	}

	tests := []struct {
		name     string
		data     PEMData
		expected string
	}{
		{"File Path", PEMData("test-file"), testPEMCert},
		{"Base64", PEMData(base64.StdEncoding.EncodeToString([]byte(testPEMCert))), testPEMCert},
		{"Raw PEM", PEMData(testPEMCert), testPEMCert},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _, err := tt.data.Load(mockStatFile, mockReadFile)
			require.NoError(t, err)
			require.Equal(t, tt.expected, string(data))
		})
	}
}

func TestStringToPEMDataHookFunc(t *testing.T) {
	hook := StringToPEMDataHookFunc().(func(
		f reflect.Type,
		t reflect.Type,
		data interface{},
	) (interface{}, error))
	result, err := hook(reflect.TypeOf(""), reflect.TypeOf(PEMData("")), "test-data")
	require.NoError(t, err)
	require.Equal(t, PEMData("test-data"), result)
}

const testPEMCert = `
-- GlobalSign Root R2, valid until Dec 15, 2021
-----BEGIN CERTIFICATE-----
MIIDujCCAqKgAwIBAgILBAAAAAABD4Ym5g0wDQYJKoZIhvcNAQEFBQAwTDEgMB4G
A1UECxMXR2xvYmFsU2lnbiBSb290IENBIC0gUjIxEzARBgNVBAoTCkdsb2JhbFNp
Z24xEzARBgNVBAMTCkdsb2JhbFNpZ24wHhcNMDYxMjE1MDgwMDAwWhcNMjExMjE1
MDgwMDAwWjBMMSAwHgYDVQQLExdHbG9iYWxTaWduIFJvb3QgQ0EgLSBSMjETMBEG
A1UEChMKR2xvYmFsU2lnbjETMBEGA1UEAxMKR2xvYmFsU2lnbjCCASIwDQYJKoZI
hvcNAQEBBQADggEPADCCAQoCggEBAKbPJA6+Lm8omUVCxKs+IVSbC9N/hHD6ErPL
v4dfxn+G07IwXNb9rfF73OX4YJYJkhD10FPe+3t+c4isUoh7SqbKSaZeqKeMWhG8
eoLrvozps6yWJQeXSpkqBy+0Hne/ig+1AnwblrjFuTosvNYSuetZfeLQBoZfXklq
tTleiDTsvHgMCJiEbKjNS7SgfQx5TfC4LcshytVsW33hoCmEofnTlEnLJGKRILzd
C9XZzPnqJworc5HGnRusyMvo4KD0L5CLTfuwNhv2GXqF4G3yYROIXJ/gkwpRl4pa
zq+r1feqCapgvdzZX99yqWATXgAByUr6P6TqBwMhAo6CygPCm48CAwEAAaOBnDCB
mTAOBgNVHQ8BAf8EBAMCAQYwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQUm+IH
V2ccHsBqBt5ZtJot39wZhi4wNgYDVR0fBC8wLTAroCmgJ4YlaHR0cDovL2NybC5n
bG9iYWxzaWduLm5ldC9yb290LXIyLmNybDAfBgNVHSMEGDAWgBSb4gdXZxwewGoG
3lm0mi3f3BmGLjANBgkqhkiG9w0BAQUFAAOCAQEAmYFThxxol4aR7OBKuEQLq4Gs
J0/WwbgcQ3izDJr86iw8bmEbTUsp9Z8FHSbBuOmDAGJFtqkIk7mpM0sYmsL4h4hO
291xNBrBVNpGP+DTKqttVCL1OmLNIG+6KYnX3ZHu01yiPqFbQfXf5WRDLenVOavS
ot+3i9DAgBkcRcAtjOj4LaR0VknFBbVPFd5uRHg5h6h+u/N5GJG79G+dwfCMNYxd
AfvDbbnvRG15RjF+Cv6pgsH/76tuIMRQyV+dTZsXjAzlAcmgQWpzU/qlULRuJQ/7
TBj0/VLZjmmx6BEP3ojY+x1J96relc8geMJgEtslQIxq/H5COEBkEveegeGTLg==
-----END CERTIFICATE-----`

func Test_isValidPEM(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected bool
	}{
		{"Valid PEM", testPEMCert, true},
		{"Invalid PEM", "/etc/passwd", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidPEM(tt.data)
			require.Equal(t, tt.expected, result)
		})
	}
}

const invalidPEM = "invalid PEM content"

func TestPEMData_Load_Invalid(t *testing.T) {
	pem := PEMData(invalidPEM)

	_, _, err := pem.Load(mockStatFileSuccess, mockReadFileInvalid)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PEM data")
}
