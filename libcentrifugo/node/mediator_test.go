package node

import (
	"testing"

	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/stretchr/testify/assert"
)

type testMediator struct {
	connect     int
	subscribe   int
	unsubscribe int
	disconnect  int
	message     int
}

func (m *testMediator) Connect(client proto.ConnID, user proto.UserID) {
	m.connect++
}

func (m *testMediator) Subscribe(ch proto.Channel, client proto.ConnID, user proto.UserID) {
	m.subscribe++
}

func (m *testMediator) Unsubscribe(ch proto.Channel, client proto.ConnID, user proto.UserID) {
	m.unsubscribe++
	return
}

func (m *testMediator) Disconnect(client proto.ConnID, user proto.UserID) {
	m.disconnect++
	return
}

func (m *testMediator) Message(ch proto.Channel, data []byte, client proto.ConnID, info *proto.ClientInfo) bool {
	m.message++
	return false
}

func TestMediator(t *testing.T) {
	app := testApp()
	m := &testMediator{}
	app.SetMediator(m)
	assert.NotEqual(t, nil, app.mediator)
}
