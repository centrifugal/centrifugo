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

func (m *testMediator) Connect(client ConnID, user UserID) {
	m.connect++
}

func (m *testMediator) Subscribe(ch Channel, client ConnID, user UserID) {
	m.subscribe++
}

func (m *testMediator) Unsubscribe(ch Channel, client ConnID, user UserID) {
	m.unsubscribe++
	return
}

func (m *testMediator) Disconnect(client ConnID, user UserID) {
	m.disconnect++
	return
}

func (m *testMediator) Message(ch Channel, data []byte, client ConnID, info *ClientInfo) bool {
	m.message++
	return false
}

func TestMediator(t *testing.T) {
	app := testApp()
	m := &testMediator{}
	app.SetMediator(m)
	assert.NotEqual(t, nil, app.mediator)
}
