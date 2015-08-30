package libcentrifugo

import (
	"testing"

	"github.com/gorilla/securecookie"
	"github.com/stretchr/testify/assert"
)

type testAdminSession struct{}

func (s *testAdminSession) WriteMessage(int, []byte) error {
	return nil
}

func newAdminTestConfig() *Config {
	return &Config{
		WebSecret: "secret",
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
	err = c.send("message")
	assert.Equal(t, nil, err)
}

func TestAdminClientMessageHandling(t *testing.T) {
	c, err := newTestAdminClient()
	assert.Equal(t, nil, err)
	emptyMsg := ""
	_, err = c.handleMessage([]byte(emptyMsg))
	assert.NotEqual(t, nil, err)
	malformedMsg := "ooops"
	_, err = c.handleMessage([]byte(malformedMsg))
	assert.NotEqual(t, nil, err)
	unknownMsg := "{\"method\":\"unknown\", \"params\": {}}"
	_, err = c.handleMessage([]byte(unknownMsg))
	assert.Equal(t, ErrInvalidMessage, err)
	emptyAuthMethod := "{\"method\":\"auth\", \"params\": {}}"
	_, err = c.handleMessage([]byte(emptyAuthMethod))
	assert.Equal(t, ErrUnauthorized, err)
	s := securecookie.New([]byte(c.app.config.WebSecret), nil)
	token, _ := s.Encode(AuthTokenKey, AuthTokenValue)
	correctAuthMethod := "{\"method\":\"auth\", \"params\": {\"token\":\"" + token + "\"}}"
	_, err = c.handleMessage([]byte(correctAuthMethod))
	assert.Equal(t, nil, err)
	pingCommand := "{\"method\":\"ping\", \"params\": {}}"
	_, err = c.handleMessage([]byte(pingCommand))
	assert.Equal(t, nil, err)
}
