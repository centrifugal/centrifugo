package sockjs

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHandler_RawWebSocketHandshakeError(t *testing.T) {
	h := newTestHandler()
	server := httptest.NewServer(http.HandlerFunc(h.rawWebsocket))
	defer server.Close()
	req, _ := http.NewRequest("GET", server.URL, nil)
	req.Header.Set("origin", "https"+server.URL[4:])
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Unexpected response code, got '%d', expected '%d'", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandler_RawWebSocket(t *testing.T) {
	h := newTestHandler()
	server := httptest.NewServer(http.HandlerFunc(h.rawWebsocket))
	defer server.CloseClientConnections()
	url := "ws" + server.URL[4:]
	var connCh = make(chan Session)
	h.handlerFunc = func(conn Session) { connCh <- conn }
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if conn == nil {
		t.Errorf("Connection should not be nil")
	}
	if err != nil {
		t.Errorf("Unexpected error '%v'", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("Wrong response code returned, got '%d', expected '%d'", resp.StatusCode, http.StatusSwitchingProtocols)
	}
	select {
	case <-connCh: //ok
	case <-time.After(10 * time.Millisecond):
		t.Errorf("Sockjs Handler not invoked")
	}
}

func TestHandler_RawWebSocketTerminationByServer(t *testing.T) {
	h := newTestHandler()
	server := httptest.NewServer(http.HandlerFunc(h.rawWebsocket))
	defer server.Close()
	url := "ws" + server.URL[4:]
	h.handlerFunc = func(conn Session) {
		// close the session without sending any message
		conn.Close(3000, "some close message")
		conn.Close(0, "this should be ignored")
	}
	conn, _, err := websocket.DefaultDialer.Dial(url, map[string][]string{"Origin": []string{server.URL}})
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	for i := 0; i < 2; i++ {
		_, _, err := conn.ReadMessage()
		closeError, ok := err.(*websocket.CloseError)
		if !ok {
			t.Fatalf("expected close error but got: %v", err)
		}
		if closeError.Code != 3000 {
			t.Errorf("unexpected close status: %v", closeError.Code)
		}
		if closeError.Text != "some close message" {
			t.Errorf("unexpected close reason: '%v'", closeError.Text)
		}
	}
}

func TestHandler_RawWebSocketTerminationByClient(t *testing.T) {
	h := newTestHandler()
	server := httptest.NewServer(http.HandlerFunc(h.rawWebsocket))
	defer server.Close()
	url := "ws" + server.URL[4:]
	var done = make(chan struct{})
	h.handlerFunc = func(conn Session) {
		if _, err := conn.Recv(); err != ErrSessionNotOpen {
			t.Errorf("Recv should fail")
		}
		close(done)
	}
	conn, _, _ := websocket.DefaultDialer.Dial(url, map[string][]string{"Origin": []string{server.URL}})
	conn.Close()
	<-done
}

func TestHandler_RawWebSocketCommunication(t *testing.T) {
	h := newTestHandler()
	server := httptest.NewServer(http.HandlerFunc(h.rawWebsocket))
	// defer server.CloseClientConnections()
	url := "ws" + server.URL[4:]
	var done = make(chan struct{})
	h.handlerFunc = func(conn Session) {
		conn.Send("message 1")
		conn.Send("message 2")
		expected := "[\"message 3\"]\n"
		msg, err := conn.Recv()
		if msg != expected || err != nil {
			t.Errorf("Got '%s', expected '%s'", msg, expected)
		}
		conn.Close(123, "close")
		close(done)
	}
	conn, _, _ := websocket.DefaultDialer.Dial(url, map[string][]string{"Origin": []string{server.URL}})
	conn.WriteJSON([]string{"message 3"})
	var expected = []string{"message 1", "message 2"}
	for _, exp := range expected {
		_, msg, err := conn.ReadMessage()
		if string(msg) != exp || err != nil {
			t.Errorf("Wrong frame, got '%s' and error '%v', expected '%s' without error", msg, err, exp)
		}
	}
	<-done
}
