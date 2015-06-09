package libcentrifugo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testAdminSession struct{}

func (s *testAdminSession) WriteMessage(int, []byte) error {
	return nil
}

func newAdminTestConfig() *config {
	return &config{
		webSecret: "secret",
	}
}

func newAdminTestApplication() *application {
	app, _ := newApplication(newAdminTestConfig())
	return app
}

func newTestAdminClient() (*adminClient, error) {
	app := newAdminTestApplication()
	c, err := newAdminClient(app, &testAdminSession{})
	return c, err
}

func TestAdminClient(t *testing.T) {
	c, err := newTestAdminClient()
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
	assert.Equal(t, ErrInvalidAdminMessage, err)
	emptyAuthMethod := "{\"method\":\"auth\", \"params\": {}}"
	_, err = c.handleMessage([]byte(emptyAuthMethod))
	assert.Equal(t, ErrUnauthorized, err)
}
