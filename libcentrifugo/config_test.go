package libcentrifugo

import (
	"testing"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

func newTestConfig() Config {
	c := *DefaultConfig
	var ns []Namespace
	ns = append(ns, getTestNamespace("test"))
	c.Namespaces = ns
	c.Secret = "secret"
	c.ChannelOptions = getTestChannelOptions()
	return c
}

func TestGetChannelOptions(t *testing.T) {
	c := newTestConfig()

	_, err := c.channelOpts("test")
	assert.Equal(t, nil, err)

	_, err = c.channelOpts("")
	assert.Equal(t, nil, err)

	_, err = c.channelOpts("wrongnamespacekey")
	assert.Equal(t, ErrNamespaceNotFound, err)
}

func TestValidate(t *testing.T) {
	c := newTestConfig()
	err := c.Validate()
	assert.Equal(t, nil, err)
}
