package logger

// import (
// 	gologger "github.com/FZambia/go-logger"
// )

// Level describes the chosen log level.
type Level int

const (
	// NONE means no logging.
	NONE Level = iota
	// TRACE enables low-level events tracing. 
	TRACE
	// DEBUG turns on debug logs - its generally too much for production.
	DEBUG
	// INFO is 
	INFO
	// WARN ...
	WARN
	// ERROR ...
	ERROR
)

// levelStringMatches has matches between Level and its string representation.
var levelStringMatches = map[Level]string{
	TRACE: "trace",
	DEBUG: "debug",
	INFO:  "info",
	WARN:  "warn",
	ERROR: "error",
}

// LevelString transforms Level to its string representation.
func LevelString(l Level) string {
	if t, ok := levelStringMatches[l]; ok {
		return t
	}
	return ""
}

// Entry represents log entry.
type Entry struct {
	Level   Level
	Message string
	Fields  map[string]interface{}
}

// NewEntry helps to create Entry.
func NewEntry(lvl Level, message string, fieldValues ...interface{}) Entry {
	if len(fieldValues)%2 != 0 {
		panic("even number of fields expected")
	}
	fields := make(map[string]interface{}, len(fieldValues)/2)
	for i := 0; i < len(fieldValues); i += 2 {
		fields[fieldValues[i].(string)] = fieldValues[i+1]
	}
	return Entry{
		Level:   lvl,
		Message: message,
		Fields:  fields,
	}
}

// Handler handles log entries - i.e. writes into correct destination if necessary.
type Handler func(Entry)

// Logger allows to log Entry.
type Logger interface {
	Log(Entry)
	Enabled(Level) bool
}

// New creates Logger.
func New(lvl Level, handler Handler) Logger {
	return &loggerImpl{
		Level:   lvl,
		Handler: handler,
	}
}

type loggerImpl struct {
	Level   Level
	Handler Handler
}

// Log ...
func (l *loggerImpl) Log(entry Entry) {
	if l.Level >= entry.Level
	l.Handler(entry)
}

func (l *loggerImpl) Enabled(lvl Level) bool {
	return l.Level >= lvl
}

// // LevelMatches is a map that matches string level to logger level constants.
// var LevelMatches = map[string]gologger.Level{
// 	"DEBUG":    gologger.LevelDebug,
// 	"INFO":     gologger.LevelInfo,
// 	"WARN":     gologger.LevelWarn,
// 	"ERROR":    gologger.LevelError,
// 	"CRITICAL": gologger.LevelCritical,
// 	"FATAL":    gologger.LevelFatal,
// 	"NONE":     gologger.LevelNone,
// }

// var (
// 	// LevelDebug level.
// 	LevelDebug = gologger.LevelDebug
// 	// LevelInfo level.
// 	LevelInfo = gologger.LevelInfo
// 	// LevelWarn level.
// 	LevelWarn = gologger.LevelWarn
// 	// LevelError level.
// 	LevelError = gologger.LevelError
// 	// LevelCritical level.
// 	LevelCritical = gologger.LevelCritical
// 	// LevelFatal level.
// 	LevelFatal = gologger.LevelFatal
// 	// LevelNone level.
// 	LevelNone = gologger.LevelNone
// )

// var (
// 	// DEBUG logger.
// 	DEBUG = gologger.DEBUG
// 	// INFO logger.
// 	INFO = gologger.INFO
// 	// WARN logger.
// 	WARN = gologger.WARN
// 	// ERROR logger.
// 	ERROR = gologger.ERROR
// 	// CRITICAL logger.
// 	CRITICAL = gologger.CRITICAL
// 	// FATAL logger
// 	FATAL = gologger.FATAL
// )

// // SetLogThreshold establishes a threshold where anything matching or above will be logged.
// func SetLogThreshold(level gologger.Level) {
// 	gologger.SetLogThreshold(level)
// }

// // SetStdoutThreshold establishes a threshold where anything matching or above will be output.
// func SetStdoutThreshold(level gologger.Level) {
// 	gologger.SetStdoutThreshold(level)
// }

// // SetLogFile sets the LogHandle to a io.writer created for the file behind the given file path.
// // Will append to this file.
// func SetLogFile(path string) error {
// 	return gologger.SetLogFile(path)
// }

// // SetLogFlag sets global log flag used in package.
// func SetLogFlag(flag int) {
// 	gologger.SetLogFlag(flag)
// }
