package logger

import (
	"bytes"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLevels(t *testing.T) {
	SetStdoutThreshold(LevelError)
	assert.Equal(t, outputThreshold, LevelError)
	SetLogThreshold(LevelCritical)
	assert.Equal(t, logThreshold, LevelCritical)
	assert.NotEqual(t, outputThreshold, LevelCritical)
	SetStdoutThreshold(LevelWarn)
	assert.Equal(t, outputThreshold, LevelWarn)
}

func BenchmarkLoggingNoop(b *testing.B) {
	// Log into io.Discard
	SetStdoutThreshold(LevelNone)
	SetLogThreshold(LevelNone)
	m := map[string]interface{}{
		"int": 1,
		"str": "test",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		INFO.Printf("%s %#v", "test", m)
	}
}

func TestSetLogFile(t *testing.T) {
	err := SetLogFile("/tmp/testing")
	assert.Equal(t, nil, err)
	err = SetLogFile("/i_want_it_to_not_exist_so_error_return/testing")
	assert.NotEqual(t, nil, err)
}

func TestSetLogFlag(t *testing.T) {
	SetLogFlag(log.Ldate)
	assert.Equal(t, Flag, log.Ldate)
}

func TestLevelCheck(t *testing.T) {
	assert.Equal(t, levelCheck(LevelDebug), LevelDebug)
	assert.Equal(t, levelCheck(LevelFatal), LevelFatal)
	assert.Equal(t, levelCheck(LevelError), LevelError)
}

func TestDefaultLogging(t *testing.T) {
	outputBuf := new(bytes.Buffer)
	logBuf := new(bytes.Buffer)
	LogHandle = logBuf
	OutHandle = outputBuf

	SetLogThreshold(LevelWarn)
	SetStdoutThreshold(LevelError)

	FATAL.Println("fatal err")
	CRITICAL.Println("critical err")
	ERROR.Println("an error")
	WARN.Println("a warning")
	INFO.Println("information")
	DEBUG.Println("debugging info")
	TRACE.Println("trace entry")

	assert.Contains(t, logBuf.String(), "fatal err")
	assert.Contains(t, logBuf.String(), "critical err")
	assert.Contains(t, logBuf.String(), "an error")
	assert.Contains(t, logBuf.String(), "a warning")
	assert.NotContains(t, logBuf.String(), "information")
	assert.NotContains(t, logBuf.String(), "debugging info")
	assert.NotContains(t, logBuf.String(), "trace entry")

	assert.Contains(t, outputBuf.String(), "fatal err")
	assert.Contains(t, outputBuf.String(), "critical err")
	assert.Contains(t, outputBuf.String(), "an error")
	assert.NotContains(t, outputBuf.String(), "a warning")
	assert.NotContains(t, outputBuf.String(), "information")
	assert.NotContains(t, outputBuf.String(), "debugging info")
	assert.NotContains(t, outputBuf.String(), "trace entry")
}
