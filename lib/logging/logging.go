package logging

// Level describes the chosen log level.
type Level int

const (
	// NONE means no logging.
	NONE Level = iota
	// DEBUG turns on debug logs - its generally too much for production in normal
	// conditions but can help when developing and investigating problems in production.
	DEBUG
	// INFO is logs useful server information.
	INFO
	// ERROR level logs only errors.
	ERROR
	// CRITICAL is logging that means non-working Centrifugo and maybe effort from
	// developers/administrators to make things work again.
	CRITICAL
)

// levelToString has matches between Level and its string representation.
var levelToString = map[Level]string{
	NONE:     "none",
	DEBUG:    "debug",
	INFO:     "info",
	ERROR:    "error",
	CRITICAL: "critical",
}

// StringToLevel ...
var StringToLevel = map[string]Level{
	"none":     NONE,
	"debug":    DEBUG,
	"info":     INFO,
	"error":    ERROR,
	"critical": CRITICAL,
}

// LevelString transforms Level to its string representation.
func LevelString(l Level) string {
	if t, ok := levelToString[l]; ok {
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
func NewEntry(level Level, message string, fields ...map[string]interface{}) Entry {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	return Entry{
		Level:   level,
		Message: message,
		Fields:  f,
	}
}

// Handler handles log entries - i.e. writes into correct destination if necessary.
type Handler func(Entry)

// New creates Logger.
func New(level Level, handler Handler) *Logger {
	return &Logger{
		level:   level,
		handler: handler,
	}
}

// Logger can log entries.
type Logger struct {
	level   Level
	handler Handler
}

// Log calls log handler with provided Entry.
func (l *Logger) Log(entry Entry) {
	if l == nil {
		return
	}
	if entry.Level >= l.level {
		l.handler(entry)
	}
}

// GetLevel returns logging level used.
func (l *Logger) GetLevel() Level {
	if l == nil {
		return NONE
	}
	return l.level
}
