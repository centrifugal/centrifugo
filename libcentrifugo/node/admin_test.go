package node

import (
	"testing"

	"github.com/gorilla/securecookie"
	"github.com/stretchr/testify/assert"
)

type testAdminSession struct{}

func (s *testAdminSession) Send([]byte) error {
	return nil
}

func (s *testAdminSession) Close(status uint32, reason string) error {
	return nil
}

func newAdminTestConfig() *Config {
	return &Config{
		AdminSecret: "secret",
	}
}

func newAdminTestNode() *Node {
	n := New(newAdminTestConfig())
	n.engine = NewTestEngine()
	return n
}

func newTestAdminClient() (AdminConn, error) {
	n := newAdminTestNode()
	c, err := n.NewAdminClient(&testAdminSession{}, nil)
	return c, err
}

func newInsecureTestAdminClient() (AdminConn, error) {
	n := newAdminTestNode()
	n.config.InsecureAdmin = true
	c, err := n.NewAdminClient(&testAdminSession{}, nil)
	return c, err
}

func TestAdminClient(t *testing.T) {
	c, err := newTestAdminClient()
	assert.Equal(t, nil, err)
	assert.NotEqual(t, c.UID(), "")
	err = c.Send([]byte("message"))
	assert.Equal(t, nil, err)
}

func TestAdminClientMessageHandling(t *testing.T) {
	c, err := newTestAdminClient()
	assert.Equal(t, nil, err)
	emptyMsg := ""
	err = c.Handle([]byte(emptyMsg))
	assert.Equal(t, nil, err)
	malformedMsg := "ooops"
	err = c.Handle([]byte(malformedMsg))
	assert.NotEqual(t, nil, err)
	emptyAuthMethod := "{\"method\":\"connect\", \"params\": {\"watch\": true}}"
	err = c.Handle([]byte(emptyAuthMethod))
	assert.Equal(t, ErrUnauthorized, err)
	s := securecookie.New([]byte("secret"), nil)
	token, _ := s.Encode(AuthTokenKey, AuthTokenValue)
	correctAuthMethod := "{\"method\":\"connect\", \"params\": {\"token\":\"" + token + "\", \"watch\": true}}"
	err = c.Handle([]byte(correctAuthMethod))
	assert.Equal(t, nil, err)
	unknownMsg := "{\"method\":\"unknown\", \"params\": {}}"
	err = c.Handle([]byte(unknownMsg))
	assert.Equal(t, ErrMethodNotFound, err)
	infoCommand := "{\"method\":\"info\", \"params\": {}}"
	err = c.Handle([]byte(infoCommand))
	assert.Equal(t, nil, err)
	pingCommand := "{\"method\":\"ping\", \"params\": {}}"
	err = c.Handle([]byte(pingCommand))
	assert.Equal(t, nil, err)
}

func TestAdminClientAuthentication(t *testing.T) {
	c, err := newTestAdminClient()
	assert.Equal(t, nil, err)
	infoCommand := "{\"method\":\"info\", \"params\": {}}"
	err = c.Handle([]byte(infoCommand))
	assert.Equal(t, ErrUnauthorized, err)
}

func TestAdminClientInsecure(t *testing.T) {
	c, err := newInsecureTestAdminClient()
	assert.Equal(t, nil, err)
	infoCommand := "{\"method\":\"info\", \"params\": {}}"
	err = c.Handle([]byte(infoCommand))
	assert.Equal(t, nil, err)
}

func TestAdminClientNotWatching(t *testing.T) {
	c, err := newTestAdminClient()
	assert.Equal(t, nil, err)
	s := securecookie.New([]byte("secret"), nil)
	token, _ := s.Encode(AuthTokenKey, AuthTokenValue)
	correctAuthMethod := "{\"method\":\"connect\", \"params\": {\"token\":\"" + token + "\"}}"
	err = c.Handle([]byte(correctAuthMethod))
	assert.Equal(t, nil, err)
	assert.Equal(t, false, c.(*adminClient).watch)
}
