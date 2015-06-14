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

func (m *testMediator) Connect(pk ProjectKey, info ClientInfo) bool {
	m.connect += 1
	return true
}

func (m *testMediator) Subscribe(pk ProjectKey, ch Channel, info ClientInfo) bool {
	m.subscribe += 1
	return true
}

func (m *testMediator) Unsubscribe(pk ProjectKey, ch Channel, info ClientInfo) {
	m.unsubscribe += 1
	return
}

func (m *testMediator) Disconnect(pk ProjectKey, info ClientInfo) {
	m.disconnect += 1
	return
}

func (m *testMediator) Message(pk ProjectKey, ch Channel, data []byte, client ConnID, info *ClientInfo, fromClient bool) bool {
	m.message += 1
	return false
}

func TestMediator(t *testing.T) {
	app := testApp()
	m := &testMediator{}
	app.SetMediator(m)
	assert.NotEqual(t, nil, app.mediator)
}
