package logger

import (
	gologger "github.com/FZambia/go-logger"
)

// LevelMatches is a map that matches string level to logger level constants.
var LevelMatches = map[string]gologger.Level{
	"TRACE":    gologger.LevelTrace,
	"DEBUG":    gologger.LevelDebug,
	"INFO":     gologger.LevelInfo,
	"WARN":     gologger.LevelWarn,
	"ERROR":    gologger.LevelError,
	"CRITICAL": gologger.LevelCritical,
	"FATAL":    gologger.LevelFatal,
	"NONE":     gologger.LevelNone,
}

var (
	// LevelTrace level.
	LevelTrace gologger.Level = gologger.LevelTrace
	// LevelDebug level.
	LevelDebug gologger.Level = gologger.LevelDebug
	// LevelInfo level.
	LevelInfo gologger.Level = gologger.LevelInfo
	// LevelWarn level.
	LevelWarn gologger.Level = gologger.LevelWarn
	// LevelError level.
	LevelError gologger.Level = gologger.LevelError
	// LevelCritical level.
	LevelCritical gologger.Level = gologger.LevelCritical
	// LevelFatal level.
	LevelFatal gologger.Level = gologger.LevelFatal
	// LevelNone level.
	LevelNone gologger.Level = gologger.LevelNone
)

var (
	// TRACE logger.
	TRACE *gologger.LevelLogger
	// DEBUG logger.
	DEBUG *gologger.LevelLogger
	// INFO logger.
	INFO *gologger.LevelLogger
	// WARN logger.
	WARN *gologger.LevelLogger
	// ERROR logger.
	ERROR *gologger.LevelLogger
	// CRITICAL logger.
	CRITICAL *gologger.LevelLogger
	// FATAL logger
	FATAL *gologger.LevelLogger
)

func init() {
	TRACE = gologger.TRACE
	DEBUG = gologger.DEBUG
	INFO = gologger.INFO
	WARN = gologger.WARN
	ERROR = gologger.ERROR
	CRITICAL = gologger.CRITICAL
	FATAL = gologger.FATAL
}

// SetLogThreshold establishes a threshold where anything matching or above will be logged.
func SetLogThreshold(level gologger.Level) {
	gologger.SetLogThreshold(level)
}

// SetStdoutThreshold establishes a threshold where anything matching or above will be output.
func SetStdoutThreshold(level gologger.Level) {
	gologger.SetStdoutThreshold(level)
}

// SetLogFile sets the LogHandle to a io.writer created for the file behind the given file path.
// Will append to this file.
func SetLogFile(path string) error {
	return gologger.SetLogFile(path)
}

// SetLogFlag sets global log flag used in package.
func SetLogFlag(flag int) {
	gologger.SetLogFlag(flag)
}
