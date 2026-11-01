package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// wsClient is a bidirectional WebSocket client speaking Centrifugal JSON
// protocol. Commands get sequential ids per client; replies are matched by
// id, everything else is buffered as pushes.
type wsClient struct {
	conn   *websocket.Conn
	nextID int
	msgBuffer
}

// msgBuffer collects what a connection receives: replies by command id,
// everything else as pushes, and how the connection ended.
type msgBuffer struct {
	mu      sync.Mutex
	replies map[int]any
	pushes  []any
	closed  *closeInfo
	notify  chan struct{}
}

func newMsgBuffer() msgBuffer {
	return msgBuffer{replies: map[int]any{}, notify: make(chan struct{}, 1)}
}

// conn is what a step can do with any client connection.
type conn interface {
	waitPushes(count int, timeout time.Duration) ([]any, error)
	takePushes() []any
	waitClose(timeout time.Duration) *closeInfo
	close()
}

// addMessage records one decoded message.
func (c *msgBuffer) addMessage(v any) {
	m, _ := v.(map[string]any)
	c.mu.Lock()
	if id, ok := m["id"].(json.Number); ok {
		n, _ := id.Int64()
		c.replies[int(n)] = v
	} else {
		c.pushes = append(c.pushes, v)
	}
	c.mu.Unlock()
	c.signal()
}

func (c *msgBuffer) setClosed(ci *closeInfo) {
	c.mu.Lock()
	if c.closed == nil {
		c.closed = ci
	}
	c.mu.Unlock()
	c.signal()
}

type closeInfo struct {
	Code   int    `json:"code"`
	Reason string `json:"reason"`
}

func dialWS(url string, headers map[string]string) (*wsClient, *http.Response, error) {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	d := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, resp, err := d.Dial(url, h)
	if err != nil {
		return nil, resp, err
	}
	c := &wsClient{conn: conn, msgBuffer: newMsgBuffer()}
	go c.readLoop()
	return c, resp, nil
}

func (c *wsClient) readLoop() {
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			ci := &closeInfo{Code: -1, Reason: "read error"}
			if ce, ok := err.(*websocket.CloseError); ok {
				ci = &closeInfo{Code: ce.Code, Reason: ce.Text}
			}
			c.setClosed(ci)
			return
		}
		// Several messages may come in one frame, separated by newlines.
		for _, line := range bytes.Split(data, []byte("\n")) {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			v, err := DecodeJSON(line)
			if err != nil {
				v = "raw:" + string(line)
			}
			m, _ := v.(map[string]any)
			if m != nil && len(m) == 0 {
				// Server ping: answer with pong, not recorded.
				_ = c.write([]byte("{}"))
				continue
			}
			c.addMessage(v)
		}
	}
}

func (c *msgBuffer) signal() {
	select {
	case c.notify <- struct{}{}:
	default:
	}
}

var writeMu sync.Mutex

func (c *wsClient) write(data []byte) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// send writes cmd with a new id. It returns the id and the encoded command.
func (c *wsClient) send(cmd map[string]any, withID bool) (int, error) {
	id := 0
	if withID {
		c.nextID++
		id = c.nextID
		cmd["id"] = id
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		return 0, err
	}
	return id, c.write(data)
}

// waitReply waits for the reply with id or for the connection close.
func (c *msgBuffer) waitReply(id int, timeout time.Duration) (any, *closeInfo) {
	deadline := time.After(timeout)
	for {
		c.mu.Lock()
		r, ok := c.replies[id]
		if ok {
			delete(c.replies, id)
		}
		closed := c.closed
		c.mu.Unlock()
		if ok {
			return r, nil
		}
		if closed != nil {
			return nil, closed
		}
		select {
		case <-c.notify:
		case <-deadline:
			return nil, &closeInfo{Code: -2, Reason: "timeout waiting for reply"}
		}
	}
}

// waitPushes waits until count pushes are buffered and takes them.
func (c *msgBuffer) waitPushes(count int, timeout time.Duration) ([]any, error) {
	deadline := time.After(timeout)
	for {
		c.mu.Lock()
		if len(c.pushes) >= count {
			out := c.pushes[:count]
			c.pushes = append([]any(nil), c.pushes[count:]...)
			c.mu.Unlock()
			return out, nil
		}
		closed := c.closed
		have := len(c.pushes)
		c.mu.Unlock()
		if closed != nil {
			return c.takePushes(), fmt.Errorf("connection closed with %d of %d pushes", have, count)
		}
		select {
		case <-c.notify:
		case <-deadline:
			return c.takePushes(), fmt.Errorf("timeout with %d of %d pushes", have, count)
		}
	}
}

func (c *msgBuffer) takePushes() []any {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.pushes
	c.pushes = nil
	return out
}

// waitClose waits for the server to close the connection.
func (c *msgBuffer) waitClose(timeout time.Duration) *closeInfo {
	deadline := time.After(timeout)
	for {
		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()
		if closed != nil {
			return closed
		}
		select {
		case <-c.notify:
		case <-deadline:
			return &closeInfo{Code: -2, Reason: "timeout waiting for close"}
		}
	}
}

func (c *wsClient) close() {
	writeMu.Lock()
	_ = c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	writeMu.Unlock()
	_ = c.conn.Close()
}

func wsURL(httpBase, path string) string {
	return "ws" + strings.TrimPrefix(httpBase, "http") + path
}
