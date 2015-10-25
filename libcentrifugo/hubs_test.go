package libcentrifugo

import (
	"fmt"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

type testClientConn struct {
	Cid      ConnID
	Uid      UserID
	Channels []Channel

	Messages [][]byte
	Closed   bool
	sess     *testSession
}

func newTestUserCC() *testClientConn {
	return &testClientConn{
		Cid:      "test uid",
		Uid:      "test user",
		Channels: []Channel{"test"},
	}
}
func (c *testClientConn) uid() ConnID {
	return c.Cid
}

func (c *testClientConn) user() UserID {
	return c.Uid
}

func (c *testClientConn) channels() []Channel {
	return c.Channels
}

func (c *testClientConn) send(message []byte) error {
	c.Messages = append(c.Messages, message)
	return nil
}

func (c *testClientConn) unsubscribe(channel Channel) error {
	for i, ch := range c.Channels {
		if ch == channel {
			c.Channels = c.Channels[:i+copy(c.Channels[i:], c.Channels[i+1:])]
			return nil
		}
	}
	return fmt.Errorf("channel '%s' not found", string(channel))
}

func (c *testClientConn) close(reason string) error {
	if c.Closed {
		return fmt.Errorf("duplicate close")
	}
	c.Closed = true
	return nil
}

type testAdminConn struct{}

func (c *testAdminConn) uid() ConnID {
	return "test uid"
}

func (c *testAdminConn) send(message string) error {
	return nil
}

func TestClientHub(t *testing.T) {
	h := newClientHub()
	c := newTestUserCC()
	h.add(c)
	assert.Equal(t, len(h.users), 1)
	conns := h.userConnections("test user")
	assert.Equal(t, 1, len(conns))
	assert.Equal(t, 1, h.nClients())
	assert.Equal(t, 1, h.nUniqueClients())
	h.remove(c)
	assert.Equal(t, len(h.users), 0)
	assert.Equal(t, 1, len(conns))
}

func TestShutdown(t *testing.T) {
	h := newClientHub()
	c := newTestUserCC()
	h.add(c)
	assert.Equal(t, len(h.users), 1)
	h.shutdown()
}

func TestSubHub(t *testing.T) {
	h := newClientHub()
	c := newTestUserCC()
	h.addSub("test1", c)
	h.addSub("test2", c)
	assert.Equal(t, 2, h.nChannels())
	channels := []string{}
	for _, ch := range h.channels() {
		channels = append(channels, string(ch))
	}
	assert.Equal(t, stringInSlice("test1", channels), true)
	assert.Equal(t, stringInSlice("test2", channels), true)
	err := h.broadcast("test1", []byte("message"))
	assert.Equal(t, err, nil)
	h.removeSub("test1", c)
	h.removeSub("test2", c)
	assert.Equal(t, len(h.subs), 0)
}

func TestAdminHub(t *testing.T) {
	h := newAdminHub()
	c := newTestUserCC()
	err := h.add(c)
	assert.Equal(t, err, nil)
	assert.Equal(t, len(h.connections), 1)
	err = h.broadcast([]byte("message"))
	assert.Equal(t, err, nil)
	err = h.remove(c)
	assert.Equal(t, err, nil)
	assert.Equal(t, len(h.connections), 0)
}

func setupHub(users, chanUser, totChannels int) (*clientHub, []*testClientConn) {
	uC := make([]*testClientConn, users)
	h := newClientHub()
	for i := range uC {
		c := newTestUserCC()
		c.Uid = UserID(fmt.Sprintf("uid-%d", i))
		c.Cid = ConnID(fmt.Sprintf("cid-%d", i))
		c.Channels = make([]Channel, 0)
		for j := 0; j < chanUser; j++ {
			ch := ChannelID(fmt.Sprintf("chan-%d", (j+i*chanUser)%totChannels))
			h.addSub(ch, c)
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
			ch := ChannelID(fmt.Sprintf("chan-%d", i%totChannels))
			h.broadcast(ch, []byte(fmt.Sprintf("message %d", i)))
			i++
		}
	})
	dur := time.Since(tn)
	b.StopTimer()
	total := 0
	for _, user := range conns {
		total += len(user.Messages)
	}
	b.Logf("Chans:%d, Msgs:%d, Rcvd:%d", h.nChannels(), b.N, total)
	if dur > time.Millisecond*10 {
		b.Logf("%d messages/sec", total*int(time.Second)/int(dur))
	}
}
