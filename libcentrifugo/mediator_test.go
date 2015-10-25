package libcentrifugo

import (
	"testing"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

type testMediator struct {
	connect     int
	subscribe   int
	unsubscribe int
	disconnect  int
	message     int
}

func (m *testMediator) Connect(client ConnID, user UserID) {
	m.connect += 1
}

func (m *testMediator) Subscribe(ch Channel, client ConnID, user UserID) {
	m.subscribe += 1
}

func (m *testMediator) Unsubscribe(ch Channel, client ConnID, user UserID) {
	m.unsubscribe += 1
	return
}

func (m *testMediator) Disconnect(client ConnID, user UserID) {
	m.disconnect += 1
	return
}

func (m *testMediator) Message(ch Channel, data []byte, client ConnID, info *ClientInfo) bool {
	m.message += 1
	return false
}

func TestMediator(t *testing.T) {
	app := testApp()
	m := &testMediator{}
	app.SetMediator(m)
	assert.NotEqual(t, nil, app.mediator)
}
