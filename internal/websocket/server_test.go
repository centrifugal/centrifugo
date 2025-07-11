// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package websocket

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

var subprotocolTests = []struct {
	h         string
	protocols []string
}{
	{"", nil},
	{"foo", []string{"foo"}},
	{"foo,bar", []string{"foo", "bar"}},
	{"foo, bar", []string{"foo", "bar"}},
	{" foo, bar", []string{"foo", "bar"}},
	{" foo, bar ", []string{"foo", "bar"}},
}

func TestSubprotocols(t *testing.T) {
	for _, st := range subprotocolTests {
		r := http.Request{Header: http.Header{"Sec-Websocket-Protocol": {st.h}}}
		protocols := Subprotocols(&r)
		if !reflect.DeepEqual(st.protocols, protocols) {
			t.Errorf("SubProtocols(%q) returned %#v, want %#v", st.h, protocols, st.protocols)
		}
	}
}

var isWebSocketUpgradeTests = []struct {
	ok bool
	h  http.Header
}{
	{false, http.Header{"Upgrade": {"websocket"}}},
	{false, http.Header{"Connection": {"upgrade"}}},
	{true, http.Header{"Connection": {"upgRade"}, "Upgrade": {"WebSocket"}}},
}

func TestIsWebSocketUpgrade(t *testing.T) {
	for _, tt := range isWebSocketUpgradeTests {
		ok := IsWebSocketUpgrade(&http.Request{Header: tt.h})
		if tt.ok != ok {
			t.Errorf("IsWebSocketUpgrade(%v) returned %v, want %v", tt.h, ok, tt.ok)
		}
	}
}

func TestSubProtocolSelection(t *testing.T) {
	upgrader := Upgrader{
		Subprotocols: []string{"foo", "bar", "baz"},
	}

	r := http.Request{Header: http.Header{"Sec-Websocket-Protocol": {"foo", "bar"}}}
	s := upgrader.selectSubprotocol(&r, nil)
	if s != "foo" {
		t.Errorf("Upgrader.selectSubprotocol returned %v, want %v", s, "foo")
	}

	r = http.Request{Header: http.Header{"Sec-Websocket-Protocol": {"bar", "foo"}}}
	s = upgrader.selectSubprotocol(&r, nil)
	if s != "bar" {
		t.Errorf("Upgrader.selectSubprotocol returned %v, want %v", s, "bar")
	}

	r = http.Request{Header: http.Header{"Sec-Websocket-Protocol": {"baz"}}}
	s = upgrader.selectSubprotocol(&r, nil)
	if s != "baz" {
		t.Errorf("Upgrader.selectSubprotocol returned %v, want %v", s, "baz")
	}

	r = http.Request{Header: http.Header{"Sec-Websocket-Protocol": {"quux"}}}
	s = upgrader.selectSubprotocol(&r, nil)
	if s != "" {
		t.Errorf("Upgrader.selectSubprotocol returned %v, want %v", s, "empty string")
	}

	upgrader = Upgrader{
		Subprotocols: nil,
	}
	r = http.Request{Header: http.Header{"Sec-Websocket-Protocol": {"foo"}}}
	s = upgrader.selectSubprotocol(&r, nil)
	if s != "" {
		t.Errorf("Upgrader.selectSubprotocol returned %v, want %v", s, "empty string")
	}
}

var checkSameOriginTests = []struct {
	ok bool
	r  *http.Request
}{
	{false, &http.Request{Host: "example.org", Header: map[string][]string{"Origin": {"https://other.org"}}}},
	{true, &http.Request{Host: "example.org", Header: map[string][]string{"Origin": {"https://example.org"}}}},
	{true, &http.Request{Host: "Example.org", Header: map[string][]string{"Origin": {"https://example.org"}}}},
}

func TestCheckSameOrigin(t *testing.T) {
	for _, tt := range checkSameOriginTests {
		ok := checkSameOrigin(tt.r)
		if tt.ok != ok {
			t.Errorf("checkSameOrigin(%+v) returned %v, want %v", tt.r, ok, tt.ok)
		}
	}
}

type reuseTestResponseWriter struct {
	brw *bufio.ReadWriter
	http.ResponseWriter
}

func (resp *reuseTestResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return fakeNetConn{strings.NewReader(""), &bytes.Buffer{}}, resp.brw, nil
}

var bufioReuseTests = []struct {
	n     int
	reuse bool
}{
	{4096, true},
	{128, false},
}

func TestBufioReuse(t *testing.T) {
	for i, tt := range bufioReuseTests {
		br := bufio.NewReaderSize(strings.NewReader(""), tt.n)
		bw := bufio.NewWriterSize(&bytes.Buffer{}, tt.n)
		resp := &reuseTestResponseWriter{
			brw: bufio.NewReadWriter(br, bw),
		}
		upgrader := Upgrader{}
		c, _, err := upgrader.Upgrade(resp, &http.Request{
			Method: http.MethodGet,
			Header: http.Header{
				"Upgrade":               []string{"websocket"},
				"Connection":            []string{"upgrade"},
				"Sec-Websocket-Key":     []string{"dGhlIHNhbXBsZSBub25jZQ=="},
				"Sec-Websocket-Version": []string{"13"},
			}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if reuse := c.br == br; reuse != tt.reuse {
			t.Errorf("%d: buffered reader reuse=%v, want %v", i, reuse, tt.reuse)
		}
		writeBuf := bufioWriterBuffer(c.NetConn(), bw)
		if reuse := &c.writeBuf[0] == &writeBuf[0]; reuse != tt.reuse {
			t.Errorf("%d: write buffer reuse=%v, want %v", i, reuse, tt.reuse)
		}
	}
}

func BenchmarkUpgrade(b *testing.B) {
	// Create a sample request with necessary WebSocket headers
	req, err := http.NewRequest("GET", "http://example.com/socket", nil)
	if err != nil {
		b.Fatal("Error creating request:", err)
	}
	req.Header.Add("Connection", "Upgrade")
	req.Header.Add("Upgrade", "websocket")
	req.Header.Add("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Add("Sec-Websocket-Version", "13")

	// Mock connection and buffer to fulfill the Hijack requirement
	conn := newMockConn()
	buf := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	// Create a response recorder with Hijack capability
	w := &HijackableResponseRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		HijackFunc: func() (net.Conn, *bufio.ReadWriter, error) {
			return conn, buf, nil
		},
	}

	//// Create a response recorder
	//w := httptest.NewRecorder()

	// Initialize the Upgrader
	upgrader := &Upgrader{
		// Configure your Upgrader here
	}

	// Pre-create the responseHeader map
	responseHeader := http.Header{}

	b.ResetTimer() // Start the benchmark timer
	for i := 0; i < b.N; i++ {
		// Run the Upgrade function
		_, _, err := upgrader.Upgrade(w, req, responseHeader)
		if err != nil {
			b.Error("Upgrade failed:", err)
		}
	}
}

func BenchmarkUpgradeSubprotocol(b *testing.B) {
	// Create a sample request with necessary WebSocket headers
	req, err := http.NewRequest("GET", "http://example.com/socket", nil)
	if err != nil {
		b.Fatal("Error creating request:", err)
	}
	req.Header.Add("Connection", "Upgrade")
	req.Header.Add("Upgrade", "websocket")
	req.Header.Add("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Add("Sec-Websocket-Version", "13")
	req.Header.Add("Sec-WebSocket-Protocol", "centrifuge-protobuf")

	// Mock connection and buffer to fulfill the Hijack requirement
	conn := newMockConn()
	buf := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	// Create a response recorder with Hijack capability
	w := &HijackableResponseRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		HijackFunc: func() (net.Conn, *bufio.ReadWriter, error) {
			return conn, buf, nil
		},
	}

	//// Create a response recorder
	//w := httptest.NewRecorder()

	// Initialize the Upgrader
	upgrader := &Upgrader{
		// Configure your Upgrader here
		Subprotocols: []string{"centrifuge-json", "centrifuge-protobuf"},
	}

	// Pre-create the responseHeader map
	responseHeader := http.Header{}

	b.ResetTimer() // Start the benchmark timer
	for i := 0; i < b.N; i++ {
		// Run the Upgrade function
		_, sub, err := upgrader.Upgrade(w, req, responseHeader)
		if err != nil {
			b.Error("Upgrade failed:", err)
		}
		if sub != "centrifuge-protobuf" {
			b.Errorf("Expected subprotocol 'centrifuge-protobuf', got '%s'", sub)
		}
	}
}

// Mock connection
func newMockConn() net.Conn {
	return &mockConn{writeBuf: io.Discard}
}

type mockConn struct {
	readBuf  bytes.Buffer
	writeBuf io.Writer
}

func (m *mockConn) LocalAddr() net.Addr {
	//TODO implement me
	panic("implement me")
}

func (m *mockConn) RemoteAddr() net.Addr {
	//TODO implement me
	panic("implement me")
}

func (m *mockConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) Read(b []byte) (int, error) {
	return m.readBuf.Read(b)
}

func (m *mockConn) Write(b []byte) (int, error) {
	return m.writeBuf.Write(b)
}

func (m *mockConn) Close() error {
	return nil
}

type HijackableResponseRecorder struct {
	*httptest.ResponseRecorder
	HijackFunc func() (net.Conn, *bufio.ReadWriter, error)
}

func (h *HijackableResponseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h.HijackFunc != nil {
		return h.HijackFunc()
	}
	return nil, nil, fmt.Errorf("Hijack not implemented")
}
