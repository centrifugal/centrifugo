package libcentrifugo

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

func newAdminTestApplication() *Application {
	app, _ := NewApplication(newAdminTestConfig())
	app.engine = newTestEngine()
	return app
}

func newTestAdminClient() (*adminClient, error) {
	app := newAdminTestApplication()
	c, err := newAdminClient(app, &testAdminSession{})
	return c, err
}

func newInsecureTestAdminClient() (*adminClient, error) {
	app := newAdminTestApplication()
	app.config.InsecureAdmin = true
	c, err := newAdminClient(app, &testAdminSession{})
	return c, err
}

func TestAdminClient(t *testing.T) {
	c, err := newTestAdminClient()
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
	emptyAuthMethod := "{\"method\":\"connect\", \"params\": {\"watch\": true}}"
	err = c.message([]byte(emptyAuthMethod))
	assert.Equal(t, ErrUnauthorized, err)
	s := securecookie.New([]byte(c.app.config.AdminSecret), nil)
	token, _ := s.Encode(AuthTokenKey, AuthTokenValue)
	correctAuthMethod := "{\"method\":\"connect\", \"params\": {\"token\":\"" + token + "\", \"watch\": true}}"
	err = c.message([]byte(correctAuthMethod))
	assert.Equal(t, nil, err)
	unknownMsg := "{\"method\":\"unknown\", \"params\": {}}"
	err = c.message([]byte(unknownMsg))
	assert.Equal(t, ErrMethodNotFound, err)
	infoCommand := "{\"method\":\"info\", \"params\": {}}"
	err = c.message([]byte(infoCommand))
	assert.Equal(t, nil, err)
	pingCommand := "{\"method\":\"ping\", \"params\": {}}"
	err = c.message([]byte(pingCommand))
	assert.Equal(t, nil, err)
}

func TestAdminClientAuthentication(t *testing.T) {
	c, err := newTestAdminClient()
	assert.Equal(t, nil, err)
	infoCommand := "{\"method\":\"info\", \"params\": {}}"
	err = c.message([]byte(infoCommand))
	assert.Equal(t, ErrUnauthorized, err)
}

func TestAdminClientInsecure(t *testing.T) {
	c, err := newInsecureTestAdminClient()
	assert.Equal(t, nil, err)
	infoCommand := "{\"method\":\"info\", \"params\": {}}"
	err = c.message([]byte(infoCommand))
	assert.Equal(t, nil, err)
}

func TestAdminClientNotWatching(t *testing.T) {
	c, err := newTestAdminClient()
	assert.Equal(t, nil, err)
	s := securecookie.New([]byte(c.app.config.AdminSecret), nil)
	token, _ := s.Encode(AuthTokenKey, AuthTokenValue)
	correctAuthMethod := "{\"method\":\"connect\", \"params\": {\"token\":\"" + token + "\"}}"
	err = c.message([]byte(correctAuthMethod))
	assert.Equal(t, nil, err)
	assert.Equal(t, false, c.watch)
}
