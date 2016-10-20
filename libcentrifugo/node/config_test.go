package node

import (
	"testing"

	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/stretchr/testify/assert"
)

func getTestChannelOptions() proto.ChannelOptions {
	return proto.ChannelOptions{
		Watch:           true,
		Publish:         true,
		Presence:        true,
		HistorySize:     1,
		HistoryLifetime: 1,
	}
}

func getTestNamespace(name NamespaceKey) Namespace {
	return Namespace{
		Name:           name,
		ChannelOptions: getTestChannelOptions(),
	}
}

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
	assert.Equal(t, proto.ErrNamespaceNotFound, err)
}

func TestValidate(t *testing.T) {
	c := newTestConfig()
	err := c.Validate()
	assert.Equal(t, nil, err)
}

func TestValidateErrorNamespaceNotUnique(t *testing.T) {
	c := *DefaultConfig
	var ns []Namespace
	ns = append(ns, getTestNamespace("test"))
	ns = append(ns, getTestNamespace("test"))
	c.Namespaces = ns
	err := c.Validate()
	assert.NotEqual(t, nil, err)
}

func TestValidateErrorNamespaceWrongName(t *testing.T) {
	c := *DefaultConfig
	var ns []Namespace
	ns = append(ns, getTestNamespace("test xwxw"))
	c.Namespaces = ns
	err := c.Validate()
	assert.NotEqual(t, nil, err)
}
