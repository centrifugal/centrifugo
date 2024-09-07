package config

import (
	"os"
	"testing"

	"github.com/centrifugal/centrifugo/v5/internal/consuming"

	"github.com/stretchr/testify/require"
)

func getConfig(t *testing.T, configFile string) (Config, Meta) {
	t.Helper()
	conf, meta, err := GetConfig(configFile)
	require.NoError(t, err)
	return conf, meta
}

func checkConfig(t *testing.T, conf Config) {
	t.Helper()
	require.NotNil(t, conf)
	require.True(t, conf.Client.Token.Enabled)
	require.Len(t, conf.Client.AllowedOrigins, 1)
	require.Equal(t, "http://localhost:3000", conf.Client.AllowedOrigins[0])
	require.Len(t, conf.Channel.Namespaces, 2)
	require.Len(t, conf.Consumers, 1)
	require.Equal(t, consuming.ConsumerType("kafka"), conf.Consumers[0].Type)
	require.Equal(t, true, conf.Consumers[0].Kafka.TLS.Enabled)
}

func TestConfigJSON(t *testing.T) {
	conf, _ := getConfig(t, "testdata/config.json")
	checkConfig(t, conf)
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
	// Set environment variables for the test
	_ = os.Setenv("CENTRIFUGO_CLIENT_ALLOWED_ORIGINS", "* http://localhost:4000")
	_ = os.Setenv("CENTRIFUGO_CLIENT_TOKEN_ENABLED", "false")
	_ = os.Setenv("CENTRIFUGO_CONSUMERS_KAFKA_KAFKA_TLS_ENABLED", "false")
	_ = os.Setenv("CENTRIFUGO_UNKNOWN_ENV", "1")
	_ = os.Setenv("CENTRIFUGO_CHANNEL_NAMESPACES", `[{"name": "env"}]`)
	defer func() {
		_ = os.Unsetenv("CENTRIFUGO_CONSUMERS_KAFKA_KAFKA_TLS_ENABLED")
		_ = os.Unsetenv("CENTRIFUGO_UNKNOWN_ENV")
		_ = os.Unsetenv("CENTRIFUGO_CLIENT_ALLOWED_ORIGINS")
		_ = os.Unsetenv("CENTRIFUGO_CLIENT_TOKEN_ENABLED")
		_ = os.Unsetenv("CENTRIFUGO_CHANNEL_NAMESPACES")
	}()
	// Proceed with the test
	conf, meta := getConfig(t, "testdata/config.json")
	require.Equal(t, false, conf.Client.Token.Enabled)
	require.Equal(t, false, conf.Consumers[0].Kafka.TLS.Enabled)
	require.Equal(t, []string{"*", "http://localhost:4000"}, conf.Client.AllowedOrigins)
	require.Len(t, conf.Channel.Namespaces, 1)
	require.Equal(t, "env", conf.Channel.Namespaces[0].Name)
	require.Contains(t, meta.UnknownEnvs, "CENTRIFUGO_UNKNOWN_ENV")
}
