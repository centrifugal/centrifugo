package libcentrifugo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testMediator struct {
	connect     int
	subscribe   int
	unsubscribe int
	disconnect  int
	message     int
}

func (m *testMediator) Connect(pk ProjectKey, client ConnID, user UserID) {
	m.connect += 1
}

func (m *testMediator) Subscribe(pk ProjectKey, ch Channel, client ConnID, user UserID) {
	m.subscribe += 1
}

func (m *testMediator) Unsubscribe(pk ProjectKey, ch Channel, client ConnID, user UserID) {
	m.unsubscribe += 1
	return
}

func (m *testMediator) Disconnect(pk ProjectKey, client ConnID, user UserID) {
	m.disconnect += 1
	return
}

func (m *testMediator) Message(pk ProjectKey, ch Channel, data []byte, client ConnID, info *ClientInfo) bool {
	m.message += 1
	return false
}

func TestMediator(t *testing.T) {
	app := testApp()
	m := &testMediator{}
	app.SetMediator(m)
	assert.NotEqual(t, nil, app.mediator)
}
