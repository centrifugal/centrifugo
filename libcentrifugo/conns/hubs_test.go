package conns

import (
	"fmt"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/stretchr/testify/assert"
)

type testClientConn struct {
	cid      proto.ConnID
	uid      proto.UserID
	channels []proto.Channel

	Messages [][]byte
	Closed   bool
	sess     *testSession
}

func newTestUserCC(cid proto.ConnID, uid proto.UserID) *testClientConn {
	return &testClientConn{
		cid:      cid,
		uid:      uid,
		channels: []proto.Channel{"test"},
	}
}
func (c *testClientConn) UID() proto.ConnID {
	return c.cid
}

func (c *testClientConn) User() proto.UserID {
	return c.uid
}

func (c *testClientConn) Channels() []proto.Channel {
	return c.channels
}

func (c *testClientConn) Send(message []byte) error {
	c.Messages = append(c.Messages, message)
	return nil
}

func (c *testClientConn) Handle(message []byte) error {
	return nil
}

func (c *testClientConn) Unsubscribe(channel proto.Channel) error {
	for i, ch := range c.Channels() {
		if ch == channel {
			c.channels = c.channels[:i+copy(c.channels[i:], c.channels[i+1:])]
			return nil
		}
	}
	return fmt.Errorf("channel '%s' not found", string(channel))
}

func (c *testClientConn) Close(reason string) error {
	if c.Closed {
		return fmt.Errorf("duplicate close")
	}
	c.Closed = true
	return nil
}

type testAdminConn struct{}

func (c *testAdminConn) uid() proto.ConnID {
	return "test uid"
}

func (c *testAdminConn) send(message string) error {
	return nil
}

func TestClientHub(t *testing.T) {
	h := newClientHub()
	c := newTestUserCC("test uid", "test user")
	h.Add(c)
	assert.Equal(t, len(h.(*clientHub).users), 1)
	conns := h.UserConnections(proto.UserID("test user"))
	assert.Equal(t, 1, len(conns))
	assert.Equal(t, 1, h.NumClients())
	assert.Equal(t, 1, h.NumUniqueClients())
	h.Remove(c)
	assert.Equal(t, len(h.(*clientHub).users), 0)
	assert.Equal(t, 1, len(conns))
}

func TestShutdown(t *testing.T) {
	h := newClientHub()
	c := newTestUserCC("test uid", "test user")
	h.Add(c)
	assert.Equal(t, len(h.(*clientHub).users), 1)
	h.Shutdown()
}

func TestSubHub(t *testing.T) {
	h := newClientHub()
	c := newTestUserCC("test uid", "test user")
	h.AddSub("test1", c)
	h.AddSub("test2", c)
	assert.Equal(t, 2, h.NumChannels())
	channels := []string{}
	for _, ch := range h.Channels() {
		channels = append(channels, string(ch))
	}
	assert.Equal(t, stringInSlice("test1", channels), true)
	assert.Equal(t, stringInSlice("test2", channels), true)
	assert.True(t, h.NumSubscribers(proto.Channel("test1")) > 0)
	assert.True(t, h.NumSubscribers(proto.Channel("test2")) > 0)
	err := h.Broadcast("test1", []byte("message"))
	assert.Equal(t, err, nil)
	h.RemoveSub("test1", c)
	h.RemoveSub("test2", c)
	assert.Equal(t, h.NumChannels(), 0)
	assert.False(t, h.NumSubscribers(proto.Channel("test1")) > 0)
	assert.False(t, h.NumSubscribers(proto.Channel("test2")) > 0)
}

func TestAdminHub(t *testing.T) {
	h := newAdminHub()
	c := newTestUserCC("test uid", "test user")
	err := h.Add(c)
	assert.Equal(t, err, nil)
	assert.Equal(t, len(h.(*adminHub).connections), 1)
	err = h.Broadcast([]byte("message"))
	assert.Equal(t, err, nil)
	err = h.Remove(c)
	assert.Equal(t, err, nil)
	assert.Equal(t, len(h.(*adminHub).connections), 0)
}

func setupHub(users, chanUser, totChannels int) (ClientHub, []*testClientConn) {
	uC := make([]*testClientConn, users)
	h := newClientHub()
	for i := range uC {
		c := newTestUserCC("test uid", "test user")
		for j := 0; j < chanUser; j++ {
			ch := proto.Channel(fmt.Sprintf("chan-%d", (j+i*chanUser)%totChannels))
			h.AddSub(ch, c)
		}
		uC[i] = c
	}
	return h, uC
}

func BenchmarkSubHubBroadCast(b *testing.B) {
	totChannels := 100
	h, conns := setupHub(50, 10, totChannels)
	b.ResetTimer()
	tn := time.Now()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ch := proto.Channel(fmt.Sprintf("chan-%d", i%totChannels))
			h.Broadcast(ch, []byte(fmt.Sprintf("message %d", i)))
			i++
		}
	})
	dur := time.Since(tn)
	b.StopTimer()
	total := 0
	for _, user := range conns {
		total += len(user.Messages)
	}
	b.Logf("Chans:%d, Msgs:%d, Rcvd:%d", h.NumChannels(), b.N, total)
	if dur > time.Millisecond*10 {
		b.Logf("%d messages/sec", total*int(time.Second)/int(dur))
	}
}
