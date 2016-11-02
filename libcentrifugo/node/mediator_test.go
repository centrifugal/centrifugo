package node

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

type testMediator struct {
	connect     int
	subscribe   int
	unsubscribe int
	disconnect  int
	message     int
}

func (m *testMediator) Connect(client string, user string) {
	m.connect++
}

func (m *testMediator) Subscribe(ch string, client string, user string) {
	m.subscribe++
}

func (m *testMediator) Unsubscribe(ch string, client string, user string) {
	m.unsubscribe++
	return
}

func (m *testMediator) Disconnect(client string, user string) {
	m.disconnect++
	return
}

func (m *testMediator) Message(ch string, data []byte, client string, info *proto.ClientInfo) bool {
	m.message++
	return false
}
