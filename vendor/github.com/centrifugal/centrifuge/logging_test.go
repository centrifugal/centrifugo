package centrifuge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogLevelToString(t *testing.T) {
	level := LogLevelToString(LogLevelDebug)
	assert.Equal(t, "debug", level)
}

type testHandler struct {
	count int
}

func (h *testHandler) Handle(e LogEntry) {
	h.count++
}

func TestLogger(t *testing.T) {
	h := testHandler{}
	l := newLogger(LogLevelError, h.Handle)
	assert.NotNil(t, l)
	l.log(newLogEntry(LogLevelDebug, "test"))
	assert.Equal(t, 0, h.count)
	l.log(newLogEntry(LogLevelError, "test"))
	assert.Equal(t, 1, h.count)
	assert.False(t, l.enabled(LogLevelDebug))
	assert.True(t, l.enabled(LogLevelError))
}

func TestNewLogEntry(t *testing.T) {
	entry := newLogEntry(LogLevelDebug, "test")
	assert.Equal(t, LogLevelDebug, entry.Level)
	assert.Equal(t, "test", entry.Message)
	assert.Nil(t, entry.Fields)

	entry = newLogEntry(LogLevelError, "test", map[string]interface{}{"one": true})
	assert.Equal(t, LogLevelError, entry.Level)
	assert.Equal(t, "test", entry.Message)
	assert.NotNil(t, entry.Fields)
	assert.Equal(t, true, entry.Fields["one"].(bool))
}
