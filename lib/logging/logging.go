package logging

// Level describes the chosen log level.
type Level int

const (
	// NONE means no logging.
	NONE Level = iota
	// TRACE enables low-level events tracing - i.e. logs as much details as
	// possible.
	TRACE
	// DEBUG turns on debug logs - its generally too much for production in
	// normal scenario but can help when developing and investigating problems
	// in production.
	DEBUG
	// INFO is logs useful server information.
	INFO
	// ERROR level logs only errors.
	ERROR
	// CRITICAL is logging that means not-working Centrifugo.
	CRITICAL
)

// levelToStringMatches has matches between Level and its string representation.
var levelToStringMatches = map[Level]string{
	NONE:     "none",
	TRACE:    "trace",
	DEBUG:    "debug",
	INFO:     "info",
	ERROR:    "error",
	CRITICAL: "critical",
}

// StringToLevelMatches ...
var StringToLevelMatches = map[string]Level{
	"none":     NONE,
	"trace":    TRACE,
	"debug":    DEBUG,
	"info":     INFO,
	"error":    ERROR,
	"critical": CRITICAL,
}

// LevelString transforms Level to its string representation.
func LevelString(l Level) string {
	if t, ok := levelToStringMatches[l]; ok {
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
func NewEntry(lvl Level, message string, fields ...map[string]interface{}) Entry {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	return Entry{
		Level:   lvl,
		Message: message,
		Fields:  f,
	}
}

// Handler handles log entries - i.e. writes into correct destination if necessary.
type Handler func(Entry)

// // Logger allows to log Entry.
// type Logger interface {
// 	Log(Entry)
// 	GetLevel() Level
// }

// New creates Logger.
func New(lvl Level, handler Handler) *Logger {
	return &Logger{
		Level:   lvl,
		Handler: handler,
	}
}

// Logger ...
type Logger struct {
	Level   Level
	Handler Handler
}

// Log ...
func (l *Logger) Log(entry Entry) {
	if l == nil {
		return
	}
	if l.Level >= entry.Level {
		l.Handler(entry)
	}
}

// GetLevel ...
func (l *Logger) GetLevel() Level {
	if l == nil {
		return NONE
	}
	return l.Level
}
