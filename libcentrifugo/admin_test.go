package libcentrifugo

import (
	"testing"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/gorilla/securecookie"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

type testAdminSession struct{}

func (s *testAdminSession) WriteMessage(int, []byte) error {
	return nil
}

func newAdminTestConfig() *Config {
	return &Config{
		AdminSecret: "secret",
	}
}

func newAdminTestApplication() *Application {
	app, _ := NewApplication(newAdminTestConfig())
	return app
}

func newTestAdminClient() (*adminClient, error) {
	app := newAdminTestApplication()
	c, err := newAdminClient(app, &testAdminSession{})
	return c, err
}

func TestAdminClient(t *testing.T) {
	c, err := newTestAdminClient()
	go c.writer()
	assert.Equal(t, nil, err)
	assert.NotEqual(t, c.uid(), "")
	err = c.send([]byte("message"))
	assert.Equal(t, nil, err)
}

func TestAdminClientMessageHandling(t *testing.T) {
	c, err := newTestAdminClient()
	assert.Equal(t, nil, err)
	emptyMsg := ""
	err = c.message([]byte(emptyMsg))
	assert.Equal(t, nil, err)
	malformedMsg := "ooops"
	err = c.message([]byte(malformedMsg))
	assert.NotEqual(t, nil, err)
	unknownMsg := "{\"method\":\"unknown\", \"params\": {}}"
	err = c.message([]byte(unknownMsg))
	assert.Equal(t, ErrMethodNotFound, err)
	emptyAuthMethod := "{\"method\":\"connect\", \"params\": {}}"
	err = c.message([]byte(emptyAuthMethod))
	assert.Equal(t, ErrUnauthorized, err)
	s := securecookie.New([]byte(c.app.config.AdminSecret), nil)
	token, _ := s.Encode(AuthTokenKey, AuthTokenValue)
	correctAuthMethod := "{\"method\":\"connect\", \"params\": {\"token\":\"" + token + "\"}}"
	err = c.message([]byte(correctAuthMethod))
	assert.Equal(t, nil, err)
	pingCommand := "{\"method\":\"ping\", \"params\": {}}"
	err = c.message([]byte(pingCommand))
	assert.Equal(t, nil, err)
}
