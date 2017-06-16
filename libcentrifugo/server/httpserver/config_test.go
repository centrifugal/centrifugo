package httpserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigValidate(t *testing.T) {
	// SSLCert and SSLKey are required when SSL == true
	fakeSert := "ssl sertificate fake value"
	fakeKey := "ssl key fake value"
	c := &Config{
		SSL:     true,
		SSLCert: fakeSert,
		SSLKey:  fakeKey,
	}
	assert.Nil(t, c.Validate())

	c.SSL = false
	c.SSLCert = ""
	c.SSLKey = ""
	assert.Nil(t, c.Validate())

	c.SSL = true
	c.SSLCert = ""
	c.SSLKey = fakeKey
	assert.Error(t, c.Validate())

	c.SSLCert = fakeSert
	c.SSLKey = ""
	assert.Error(t, c.Validate())
}

const SOCKSJS_HEARTBEAT_DELAY = 1

type fakeConfigGetter struct {
	autocertHostWhitelist string
}

// Get, IsSet and UnmarshalKey are not used in test cases
func (c *fakeConfigGetter) Get(key string) interface{}                        { return nil }
func (c *fakeConfigGetter) IsSet(key string) bool                             { return false }
func (c *fakeConfigGetter) UnmarshalKey(key string, target interface{}) error { return nil }

func (c *fakeConfigGetter) GetString(key string) string {
	if key == "ssl_autocert_host_whitelist" {
		return c.autocertHostWhitelist
	}
	if key == "web_path" {
		return "web_path_value"
	}
	return ""
}

func (c *fakeConfigGetter) GetBool(key string) bool {
	if key == "web" {
		return true
	}
	return false
}

func (c *fakeConfigGetter) GetInt(key string) int {
	if key == "sockjs_heartbeat_delay" {
		return SOCKSJS_HEARTBEAT_DELAY
	}
	return -1
}

func TestNewConfig(t *testing.T) {
	cfg := newConfig(&fakeConfigGetter{})
	assert.True(t, cfg.Web)
	assert.Equal(t, cfg.WebPath, "web_path_value")
	assert.Equal(t, cfg.SockjsHeartbeatDelay, SOCKSJS_HEARTBEAT_DELAY)
	assert.Nil(t, cfg.SSLAutocertHostWhitelist)

	cfg = newConfig(&fakeConfigGetter{autocertHostWhitelist: "one,two,three"})
	assert.Equal(t, cfg.SSLAutocertHostWhitelist, []string{"one", "two", "three"})
}
