package configtypes

import (
	"crypto/tls"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// Mock file-related functions
func mockReadFileSuccess(name string) ([]byte, error) {
	return []byte(`-----BEGIN CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJAO7rZ8LQQR6cMA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
...
-----END CERTIFICATE-----`), nil
}

func mockReadFileInvalid(name string) ([]byte, error) {
	return []byte("invalid content"), nil
}

func mockStatFileSuccess(name string) (os.FileInfo, error) {
	return nil, nil // Mock successful stat
}

// Sample valid PEM strings
const validPEM = `-----BEGIN CERTIFICATE-----
MIIEbjCCAtagAwIBAgIRAN1ZJEYl5ZNIOHsbJizQpucwDQYJKoZIhvcNAQELBQAw
gZ8xHjAcBgNVBAoTFW1rY2VydCBkZXZlbG9wbWVudCBDQTE6MDgGA1UECwwxZnpA
TWFjQm9vay1Qcm8tQWxleGFuZGVyLmxvY2FsIChBbGV4YW5kZXIgRW1lbGluKTFB
MD8GA1UEAww4bWtjZXJ0IGZ6QE1hY0Jvb2stUHJvLUFsZXhhbmRlci5sb2NhbCAo
QWxleGFuZGVyIEVtZWxpbikwHhcNMjIwNjE2MDYxOTM0WhcNMjQwOTE2MDYxOTM0
WjBlMScwJQYDVQQKEx5ta2NlcnQgZGV2ZWxvcG1lbnQgY2VydGlmaWNhdGUxOjA4
BgNVBAsMMWZ6QE1hY0Jvb2stUHJvLUFsZXhhbmRlci5sb2NhbCAoQWxleGFuZGVy
IEVtZWxpbikwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDLCNVIle5k
lfRtzjHe9sEo8zU9pqXfK9fxc2PZqfd6HVDVWyrOHNv9zWV8awEEgwX2kg+sY4ch
uKmNdD19UWxLovCMkA92gKhzJoPPBMlVRtSA9QWNw4cXXB25KErPPyBXyyFA13X/
6N408I26Aj6ewA0WLISkNgiCddUo31FygTNH4yWXF+F+lol0EJhG+K3E8diYub4P
1Ul417sQ/1FxcoGo43fGl8j4y6wCnBQkSNaQCr1vvNEzdmiIYF02a51Efdb3PrSu
90nJJBbFQxNhpcl98tLRF5t3wZJ+R2Xy4xPUZYwNNWTdICqW7a4bfD4foByp85kr
u44kw7laXghhAgMBAAGjXjBcMA4GA1UdDwEB/wQEAwIFoDATBgNVHSUEDDAKBggr
BgEFBQcDATAfBgNVHSMEGDAWgBSMh55IrbevJTB4kiFUXsarEAIjXjAUBgNVHREE
DTALgglsb2NhbGhvc3QwDQYJKoZIhvcNAQELBQADggGBAG9yTMOybS6ike/UoIrj
xLE3a9nPuFdt9anS0XgYicCYNFLc5H6MUXubsqBz30zigFbNP/FR2DKvIP+1cySP
DKqnimTdxZWjzT9d0YHEYcD971yk/whXmKOcla2VmYMuPmUr6M3BmUmYcoWve/ML
nc8qKJ+CsM80zxFSRbqCVqgPfNDzPHqGbJmOn0KbLPWzkUsIbii/O4IjqycJiDMS
Cyuat2Q8TYGiRhDJnouD/semDtqaIGGT77/5QLoEhFRwRKbOfgTT0hjLgTbeKPrx
QKARxjVC/QF59nhdf+je/BgrF7jfR1UuCSxwl0xg2Ub2JB5A77efWEoQh2fuSgZk
mVTZqDnfGvfYcGE9oiAMl21DimEAdYFSAUTtVI6T0S8BagN3jD+FLV7+TJgPiyIO
Lz9gcDP1Zn3jIp4Vy2HawWt+8rta351L70ie9Sk6Cx5fV0slvTFteWYdm26BuKbp
NF7OqlGSRzM2iEVaMFLqnrRwDF4bR7qwGukppEXPrsAq2Q==
-----END CERTIFICATE-----`

const validKey = `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDLCNVIle5klfRt
zjHe9sEo8zU9pqXfK9fxc2PZqfd6HVDVWyrOHNv9zWV8awEEgwX2kg+sY4chuKmN
dD19UWxLovCMkA92gKhzJoPPBMlVRtSA9QWNw4cXXB25KErPPyBXyyFA13X/6N40
8I26Aj6ewA0WLISkNgiCddUo31FygTNH4yWXF+F+lol0EJhG+K3E8diYub4P1Ul4
17sQ/1FxcoGo43fGl8j4y6wCnBQkSNaQCr1vvNEzdmiIYF02a51Efdb3PrSu90nJ
JBbFQxNhpcl98tLRF5t3wZJ+R2Xy4xPUZYwNNWTdICqW7a4bfD4foByp85kru44k
w7laXghhAgMBAAECggEAdK4z3E4FvZqL6RrJgEgwg7cZTr/ZrWKF7EWTCYDrLytv
y91jwSXGq5oBi7n20L/3ilcwWLKt8wwrrJYzzDQh12nhcfZMXJ7dr6dfsnYeujpF
X4LwWSMYHK2ci08DhwzRKoMbLidksdgC80uXN2GY2SSnoKme5LwEsezDvoRwSyu7
nQCsCqJ0hN41s04vlva8FmgNNOhavvIdR2FNYv3KhnAf/Wv1GfHzq/2kU8j46v6T
/hcZ3m+LC2J982+dyyCp+9+1Grr7rGRHiKXABcpUubAgdnwKS74oyq998/NMEoz3
OL0j1D6FxpCshLo3RP97nfnbJfguUL8eQ7uB1DR67QKBgQD7hpWGTV4urE8rLDb8
YANthNMFtvB06ZGKK+R/HEewIt4dGqbwpBcp95Bvmjxa2oA26B8tWcKkIghWQmqx
97icluXdo/x/FZvv5JZLkqZBkRzJFdCpAkbNyWmswo5YI9pzBd370fZqBN+KwhdB
LPa3UML+RXPMD+EDRen3ryXniwKBgQDOpW0gZ26mhDO7/TRuzx5I7DIFfl4lIuMt
1P9T7yY1fwnpgOvV7zl96PaqGMVSh5DvV0IR8PNVup0NAYk0qEMgyLdc+ZAu5r4N
euQEeS9HzIThqoEBSq0u0ds73QirIQ+k8r39q1XABZfNvIauhAToQxdgQgPjQtiH
ytygvR0tQwKBgB8cNF5aL24CbgBfBaYNkh73sMoiKHetdAztBOQb8Vn91g8vfrqA
8USFlF3Za+Go6PbhmwmW8pYuh21z5ZKBm1ny6BeT8uUdHR583YIXb2zor/DHO/nL
iEpnwSRXJBgOxzQ245AEFkBiveuBujKbhyCBYrzkhkAVLrWi7h9ukHelAoGBALOI
UZj3g9Czxuaqg6VJ2LvuST8wnMaS2uD0zqezfHS53Hi8Ayko38AeaD87qiObmDX4
j3Ra7G4s5UlpbjULgta2y2fBgpzc5316qSOhzYwJieEta0sd//xPYrNNw7w5ywe5
xYrgEm3z7gFWq4RvOnw33dVJRWtqpgjEHI6h/vlVAoGAG0LyA8WuCp6wUcUUzy2U
n0Ia8Dh5hCy8GvxslMgUy7t0efIkMV1dJ0IuefULUm5ItsGwqRMfLw05RE78ZoE1
Jr18O4VLqXHDDYbamXO5koEljIeQ0Y8oAKJcB+f0w9bf70RpP/Bsi+fyHrOQHWBE
uNfa+M0tCJMV+XiRsjqDNd8=
-----END PRIVATE KEY-----`

// 🧠 Test for TLSConfig.ToGoTLSConfig
func TestToGoTLSConfig_Disabled(t *testing.T) {
	cfg := TLSConfig{
		Enabled: false,
	}
	tlsCfg, err := cfg.ToGoTLSConfig("test-entity")
	require.NoError(t, err)
	require.Nil(t, tlsCfg)
}

func TestToGoTLSConfig_Valid(t *testing.T) {
	logger := zerolog.Nop()

	cfg := TLSConfig{
		Enabled:     true,
		CertPem:     PEMData(validPEM),
		KeyPem:      PEMData(validKey),
		ServerCAPem: PEMData(validPEM),
		ClientCAPem: PEMData(validPEM),
		ServerName:  "test.server",
	}

	tlsCfg, err := makeTLSConfig(cfg, logger, mockReadFileSuccess, mockStatFileSuccess)
	require.NoError(t, err)
	require.NotNil(t, tlsCfg)
	require.Equal(t, "test.server", tlsCfg.ServerName)
	require.False(t, tlsCfg.InsecureSkipVerify)
}

func TestMakeTLSConfig_InvalidCertKey(t *testing.T) {
	logger := zerolog.Nop()

	cfg := TLSConfig{
		Enabled: true,
		CertPem: PEMData(invalidPEM),
		KeyPem:  PEMData(invalidPEM),
	}

	_, err := makeTLSConfig(cfg, logger, mockReadFileSuccess, mockStatFileSuccess)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error load certificate")
}

func TestMakeTLSConfig_InsecureSkipVerify(t *testing.T) {
	logger := zerolog.Nop()

	cfg := TLSConfig{
		Enabled:            true,
		CertPem:            PEMData(validPEM),
		KeyPem:             PEMData(validKey),
		InsecureSkipVerify: true,
	}

	tlsCfg, err := makeTLSConfig(cfg, logger, mockReadFileSuccess, mockStatFileSuccess)
	require.NoError(t, err)
	require.NotNil(t, tlsCfg)
	require.True(t, tlsCfg.InsecureSkipVerify)
}

func TestLoadCertificate_NoPEM(t *testing.T) {
	logger := zerolog.Nop()
	tlsCfg := &tls.Config{}

	err := loadCertificate(TLSConfig{}, logger, tlsCfg, mockReadFileSuccess, mockStatFileSuccess)
	require.NoError(t, err)
	require.Empty(t, tlsCfg.Certificates)
}

func TestLoadServerCA_NoPEM(t *testing.T) {
	logger := zerolog.Nop()
	tlsCfg := &tls.Config{}

	err := loadServerCA(TLSConfig{}, logger, tlsCfg, mockReadFileSuccess, mockStatFileSuccess)
	require.NoError(t, err)
	require.Nil(t, tlsCfg.RootCAs)
}

func TestLoadClientCA_NoPEM(t *testing.T) {
	logger := zerolog.Nop()
	tlsCfg := &tls.Config{}

	err := loadClientCA(TLSConfig{}, logger, tlsCfg, mockReadFileSuccess, mockStatFileSuccess)
	require.NoError(t, err)
	require.Nil(t, tlsCfg.ClientCAs)
}

func TestNewCertPoolFromPEM_Invalid(t *testing.T) {
	_, err := newCertPoolFromPEM([]byte("invalid"))
	require.Error(t, err)
}

func TestNewCertPoolFromPEM_Valid(t *testing.T) {
	_, err := newCertPoolFromPEM([]byte(validPEM))
	require.NoError(t, err)
}
