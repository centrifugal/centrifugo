package websocket

import (
	"bytes"
	"testing"
	"time"
)

// TestCloseCode_Unset returns zero values before any close frame is observed.
func TestCloseCode_Unset(t *testing.T) {
	c := newTestConn(nil, &bytes.Buffer{}, true)
	if code, incoming := c.CloseCode(); code != 0 || incoming {
		t.Fatalf("CloseCode() = (%d, %v), want (0, false)", code, incoming)
	}
}

// TestCloseCode_Outgoing records a close code we write as outgoing.
func TestCloseCode_Outgoing(t *testing.T) {
	c := newTestConn(nil, &bytes.Buffer{}, true)
	_ = c.WriteControl(CloseMessage, FormatCloseMessage(CloseMessageTooBig, ""), time.Now().Add(time.Second))
	if code, incoming := c.CloseCode(); code != CloseMessageTooBig || incoming {
		t.Fatalf("CloseCode() = (%d, %v), want (%d, false)", code, incoming, CloseMessageTooBig)
	}
}

// TestCloseCode_Incoming records a close code received from the peer as incoming
// (and the subsequent RFC echo must not flip it to outgoing).
func TestCloseCode_Incoming(t *testing.T) {
	var buf bytes.Buffer
	// Client writes a close frame (masked) into buf.
	wc := newTestConn(nil, &buf, false)
	_ = wc.WriteControl(CloseMessage, FormatCloseMessage(CloseGoingAway, ""), time.Now().Add(time.Second))

	// Server reads it; advanceFrame records it as incoming and echoes a close.
	rc := newTestConn(&buf, &bytes.Buffer{}, true)
	if _, _, err := rc.NextReader(); !IsCloseError(err, CloseGoingAway) {
		t.Fatalf("NextReader err = %v, want CloseError(%d)", err, CloseGoingAway)
	}
	if code, incoming := rc.CloseCode(); code != CloseGoingAway || !incoming {
		t.Fatalf("CloseCode() = (%d, %v), want (%d, true)", code, incoming, CloseGoingAway)
	}
}

// TestCloseCode_FirstWins keeps the initiating close, ignoring later frames.
func TestCloseCode_FirstWins(t *testing.T) {
	c := newTestConn(nil, &bytes.Buffer{}, true)
	deadline := time.Now().Add(time.Second)
	_ = c.WriteControl(CloseMessage, FormatCloseMessage(CloseMessageTooBig, ""), deadline)
	_ = c.WriteControl(CloseMessage, FormatCloseMessage(CloseNormalClosure, ""), deadline)
	if code, incoming := c.CloseCode(); code != CloseMessageTooBig || incoming {
		t.Fatalf("CloseCode() = (%d, %v), want (%d, false)", code, incoming, CloseMessageTooBig)
	}
}

// TestCloseCode_IncomingAppRangeStillIncoming proves an attacker-supplied
// application-range close code (3000-4999) is recorded as incoming, so the
// Centrifuge layer (which only records !incoming) will not turn it into a label.
func TestCloseCode_IncomingAppRangeStillIncoming(t *testing.T) {
	const appCode = 4321
	var buf bytes.Buffer
	wc := newTestConn(nil, &buf, false)
	_ = wc.WriteControl(CloseMessage, FormatCloseMessage(appCode, ""), time.Now().Add(time.Second))

	rc := newTestConn(&buf, &bytes.Buffer{}, true)
	if _, _, err := rc.NextReader(); !IsCloseError(err, appCode) {
		t.Fatalf("NextReader err = %v, want CloseError(%d)", err, appCode)
	}
	code, incoming := rc.CloseCode()
	if code != appCode || !incoming {
		t.Fatalf("CloseCode() = (%d, %v), want (%d, true)", code, incoming, appCode)
	}
}
