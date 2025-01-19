package config

import (
	"os"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/stretchr/testify/require"
)

func getConfig(t *testing.T, configFile string) (Config, Meta) {
	t.Helper()
	conf, meta, err := GetConfig(nil, configFile)
	require.NoError(t, err)
	return conf, meta
}

func checkConfig(t *testing.T, conf Config) {
	t.Helper()
	require.NotNil(t, conf)
	require.True(t, conf.HTTP.TLS.Enabled)
	require.NotZero(t, conf.HTTP.TLS.ServerCAPem)
	_, err := conf.HTTP.TLS.ToGoTLSConfig("test")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/jwks", conf.Client.Token.JWKSPublicEndpoint)
	require.Len(t, conf.Client.AllowedOrigins, 1)
	require.Equal(t, "http://localhost:3000", conf.Client.AllowedOrigins[0])
	require.Len(t, conf.Channel.Namespaces, 2)
	require.Len(t, conf.Consumers, 1)
	require.Equal(t, "kafka", conf.Consumers[0].Type)
	require.Equal(t, "ppp", conf.Proxies[0].Name)
	require.Equal(t, configtypes.Duration(time.Second), conf.Proxies[0].Timeout)
	require.Equal(t, true, conf.Consumers[0].Kafka.TLS.Enabled)
	require.Equal(t, configtypes.Duration(2*time.Second), conf.WebSocket.WriteTimeout)
	require.Equal(t, "redis", conf.Engine.Type)
	require.Equal(t, 30*time.Second, time.Duration(conf.Engine.Redis.PresenceTTL))
	require.Equal(t, []string{"redis:6379"}, conf.Engine.Redis.Address)
}

func TestConfigJSON(t *testing.T) {
	conf, meta := getConfig(t, "testdata/config.json")
	checkConfig(t, conf)
	t.Log("unknown keys", meta.UnknownKeys)
	t.Log("unknown keys", meta.UnknownEnvs)
	require.Len(t, meta.UnknownKeys, 0)
	require.Len(t, meta.UnknownEnvs, 0)
}

func TestConfigYAML(t *testing.T) {
	conf, _ := getConfig(t, "testdata/config.yaml")
	checkConfig(t, conf)
}

func TestConfigTOML(t *testing.T) {
	conf, _ := getConfig(t, "testdata/config.toml")
	checkConfig(t, conf)
}

func TestConfigEnvVars(t *testing.T) {
	// Set environment variables for the test.
	_ = os.Setenv("CENTRIFUGO_CLIENT_ALLOWED_ORIGINS", "* http://localhost:4000")
	_ = os.Setenv("CENTRIFUGO_CLIENT_TOKEN_JWKS_PUBLIC_ENDPOINT", "https://example.com/jwks/new")
	_ = os.Setenv("CENTRIFUGO_CONSUMERS_KAFKA_KAFKA_TLS_ENABLED", "false")
	_ = os.Setenv("CENTRIFUGO_UNKNOWN_ENV", "1")
	_ = os.Setenv("CENTRIFUGO_CHANNEL_NAMESPACES", `[{"name": "env"}]`)
	_ = os.Setenv("CENTRIFUGO_CLIENT_PROXY_CONNECT_HTTP_STATIC_HEADERS", `{"key": "value"}`)
	_ = os.Setenv("CENTRIFUGO_WEBSOCKET_WRITE_TIMEOUT", `300ms`)
	_ = os.Setenv("CENTRIFUGO_PROXIES", `[]`)
	_ = os.Setenv("CENTRIFUGO_ENABLE_UNRELEASED_FEATURES", ``)   // Empty options should be treated as unset.
	_ = os.Setenv("CENTRIFUGO_RPC_NAMESPACES", ``)               // Empty options should be treated as unset.
	_ = os.Setenv("CENTRIFUGO_CLIENT_PROXY_CONNECT_TIMEOUT", ``) // Empty unset value should inherit default.
	defer func() {
		_ = os.Unsetenv("CENTRIFUGO_CONSUMERS_KAFKA_KAFKA_TLS_ENABLED")
		_ = os.Unsetenv("CENTRIFUGO_UNKNOWN_ENV")
		_ = os.Unsetenv("CENTRIFUGO_CLIENT_ALLOWED_ORIGINS")
		_ = os.Unsetenv("CENTRIFUGO_CLIENT_TOKEN_JWKS_PUBLIC_ENDPOINT")
		_ = os.Unsetenv("CENTRIFUGO_CHANNEL_NAMESPACES")
		_ = os.Unsetenv("CENTRIFUGO_CLIENT_PROXY_CONNECT_HTTP_STATIC_HEADERS")
		_ = os.Unsetenv("CENTRIFUGO_WEBSOCKET_WRITE_TIMEOUT")
		_ = os.Unsetenv("CENTRIFUGO_PROXIES")
		_ = os.Unsetenv("CENTRIFUGO_ENABLE_UNRELEASED_FEATURES")
		_ = os.Unsetenv("CENTRIFUGO_RPC_NAMESPACES")
		_ = os.Unsetenv("CENTRIFUGO_CLIENT_PROXY_CONNECT_TIMEOUT")
	}()
	// Proceed with the test.
	conf, meta := getConfig(t, "testdata/config.json")
	require.Equal(t, "https://example.com/jwks/new", conf.Client.Token.JWKSPublicEndpoint)
	require.Equal(t, false, conf.Consumers[0].Kafka.TLS.Enabled)
	require.Equal(t, []string{"*", "http://localhost:4000"}, conf.Client.AllowedOrigins)
	require.Len(t, conf.Channel.Namespaces, 1)
	require.Equal(t, "env", conf.Channel.Namespaces[0].Name)
	require.Len(t, meta.UnknownEnvs, 1)
	require.Len(t, meta.UnknownKeys, 0)
	require.Contains(t, meta.UnknownEnvs, "CENTRIFUGO_UNKNOWN_ENV")
	require.Equal(t, configtypes.MapStringString(map[string]string{"key": "value"}), conf.Client.Proxy.Connect.HTTP.StaticHeaders)
	require.Equal(t, configtypes.Duration(300*time.Millisecond), conf.WebSocket.WriteTimeout)
	require.Equal(t, time.Second, time.Duration(conf.Client.Proxy.Connect.Timeout))
	require.Len(t, conf.RPC.Namespaces, 0)
	require.Len(t, conf.Proxies, 0)
}

func TestConfigEnvVarsNamedArray(t *testing.T) {
	_ = os.Setenv("CENTRIFUGO_RPC_NAMESPACES", `rpc1 rpc2`)
	_ = os.Setenv("CENTRIFUGO_RPC_NAMESPACES_RPC1_PROXY_ENABLED", `true`)
	_ = os.Setenv("CENTRIFUGO_RPC_NAMESPACES_RPC1_PROXY_NAME", `custom`)
	defer func() {
		_ = os.Unsetenv("CENTRIFUGO_RPC_NAMESPACES")
		_ = os.Unsetenv("CENTRIFUGO_RPC_NAMESPACES_RPC1_PROXY_ENABLED")
		_ = os.Unsetenv("CENTRIFUGO_RPC_NAMESPACES_RPC1_PROXY_NAME")
	}()
	conf, meta := getConfig(t, "testdata/config.json")
	require.Len(t, meta.UnknownEnvs, 0)
	require.Len(t, conf.RPC.Namespaces, 2)
	require.Equal(t, "rpc1", conf.RPC.Namespaces[0].Name)
	require.Equal(t, "rpc2", conf.RPC.Namespaces[1].Name)
	require.Equal(t, true, conf.RPC.Namespaces[0].ProxyEnabled)
	require.Equal(t, "custom", conf.RPC.Namespaces[0].ProxyName)
}

// TestIsKubernetesEnvVar validates the isKubernetesEnvVar function.
func TestIsKubernetesEnvVar(t *testing.T) {
	tests := []struct {
		envKey   string
		expected bool
	}{
		{"CENTRIFUGO_PORT_11000_TCP", true},
		{"CENTRIFUGO_SERVICE_", true},
		{"CENTRIFUGO_PORT_11000", true},
		{"CENTRIFUGO_SERVICE_22000", true},
		{"CENTRIFUGO_APP_PORT_", true},
		{"CENTRIFUGO_APP_SERVICE_", true},
		{"CENTRIFUGO_APP_SERVICE_PORT_INTERNAL", true},
		{"CENTRIFUGO_APP_PORT", true},

		{"FOO_CENTRIFUGO_PORT_", false},
		{"CENTRIFUGO__PORT_", false},
		{"CENTRIFUGO_INVALID_", false},
		{"CENTRIFUGO_APP", false},
		{"CENTRIFUGO_APP_INVALID_", false},
		{"centrifugo_port_", false},

		{"CENTRIFUGO_LOG_FILE", false},
	}

	for _, tt := range tests {
		t.Run(tt.envKey, func(t *testing.T) {
			result := isKubernetesEnvVar(tt.envKey)
			require.Equal(t, tt.expected, result, "Test failed for input: %s", tt.envKey)
		})
	}
}
