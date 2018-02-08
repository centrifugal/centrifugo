package logger

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sync/atomic"
)

// Level describes the chosen log level
type Level int

// LevelLogger represents levelled logger.
type LevelLogger struct {
	enabled int32
	level   Level
	prefix  string
	logger  *log.Logger
}

// Enabled exists to prevent calling underlying logger methods when not needed.
// This can also called from library users before calling LevelLogger methods
// to reduce allocations.
func (n *LevelLogger) Enabled() bool {
	return atomic.LoadInt32(&n.enabled) != 0
}

var callDepth = 2

// Write allows LevelLogger to implement io.Writer interface so we can use it
// as output for other loggers.
func (n *LevelLogger) Write(p []byte) (int, error) {
	if !n.Enabled() {
		return len(p), nil
	}
	n.Printf("%s", p)
	return len(p), nil
}

// Print calls underlying Logger Print func.
func (n *LevelLogger) Print(v ...interface{}) {
	if !n.Enabled() {
		return
	}
	n.logger.Output(callDepth, fmt.Sprint(v...))
}

// Printf calls underlying Logger Printf func.
func (n *LevelLogger) Printf(format string, v ...interface{}) {
	if !n.Enabled() {
		return
	}
	n.logger.Output(callDepth, fmt.Sprintf(format, v...))
}

// Println calls underlying Logger Println func.
func (n *LevelLogger) Println(v ...interface{}) {
	if !n.Enabled() {
		return
	}
	n.logger.Output(callDepth, fmt.Sprintln(v...))
}

// Fatal calls underlying Logger Fatal func.
func (n *LevelLogger) Fatal(v ...interface{}) {
	if !n.Enabled() {
		return
	}
	n.logger.Output(callDepth, fmt.Sprint(v...))
	os.Exit(1)
}

// Fatalf calls underlying Logger Fatalf func.
func (n *LevelLogger) Fatalf(format string, v ...interface{}) {
	if !n.Enabled() {
		return
	}
	n.logger.Output(callDepth, fmt.Sprintf(format, v...))
	os.Exit(1)
}

// Fatalln calls underlying Logger Fatalln func.
func (n *LevelLogger) Fatalln(v ...interface{}) {
	if !n.Enabled() {
		return
	}
	n.logger.Output(callDepth, fmt.Sprintln(v...))
	os.Exit(1)
}

// Panic calls underlying Logger Panic func.
func (n *LevelLogger) Panic(v ...interface{}) {
	if !n.Enabled() {
		return
	}
	s := fmt.Sprint(v...)
	n.logger.Output(callDepth, s)
	panic(s)
}

// Panicf calls underlying Logger Panicf func.
func (n *LevelLogger) Panicf(format string, v ...interface{}) {
	if !n.Enabled() {
		return
	}
	s := fmt.Sprintf(format, v...)
	n.logger.Output(callDepth, s)
	panic(s)
}

// Panicln calls underlying Logger Panicln func.
func (n *LevelLogger) Panicln(v ...interface{}) {
	if !n.Enabled() {
		return
	}
	s := fmt.Sprintln(v...)
	n.logger.Output(callDepth, s)
	panic(s)
}

const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelCritical
	LevelFatal
	LevelNone

	DefaultLogThreshold    = LevelInfo
	DefaultStdoutThreshold = LevelInfo
)

var (
	logger *log.Logger

	LogHandle  io.Writer = ioutil.Discard
	OutHandle  io.Writer = os.Stdout
	BothHandle io.Writer = io.MultiWriter(LogHandle, OutHandle)

	Flag int = log.Ldate | log.Ltime

	TRACE    *LevelLogger = &LevelLogger{level: LevelTrace, logger: logger, prefix: "[T]: "}
	DEBUG    *LevelLogger = &LevelLogger{level: LevelDebug, logger: logger, prefix: "[D]: "}
	INFO     *LevelLogger = &LevelLogger{level: LevelInfo, logger: logger, prefix: "[I]: "}
	WARN     *LevelLogger = &LevelLogger{level: LevelWarn, logger: logger, prefix: "[W]: "}
	ERROR    *LevelLogger = &LevelLogger{level: LevelError, logger: logger, prefix: "[E]: "}
	CRITICAL *LevelLogger = &LevelLogger{level: LevelCritical, logger: logger, prefix: "[C]: "}
	FATAL    *LevelLogger = &LevelLogger{level: LevelFatal, logger: logger, prefix: "[F]: "}

	loggers []*LevelLogger = []*LevelLogger{TRACE, DEBUG, INFO, WARN, ERROR, CRITICAL, FATAL}

	logThreshold    Level = DefaultLogThreshold
	outputThreshold Level = DefaultStdoutThreshold
)

var LevelMatches = map[string]Level{
	"TRACE":    LevelTrace,
	"DEBUG":    LevelDebug,
	"INFO":     LevelInfo,
	"WARN":     LevelWarn,
	"ERROR":    LevelError,
	"CRITICAL": LevelCritical,
	"FATAL":    LevelFatal,
	"NONE":     LevelNone,
}

func init() {
	initialize()
}

// initialize initializes loggers.
func initialize() {
	BothHandle = io.MultiWriter(LogHandle, OutHandle)
	for _, l := range loggers {

		var handler io.Writer
		var enabled int32

		if l.level < outputThreshold && l.level < logThreshold {
			enabled = 0
			handler = ioutil.Discard
		} else if l.level >= outputThreshold && l.level >= logThreshold {
			enabled = 1
			handler = BothHandle
		} else if l.level >= outputThreshold && l.level < logThreshold {
			enabled = 1
			handler = OutHandle
		} else {
			enabled = 1
			handler = LogHandle
		}

		atomic.StoreInt32(&l.enabled, 0)
		l.logger = log.New(handler, l.prefix, Flag)
		atomic.StoreInt32(&l.enabled, enabled)
	}
}

// Ensures that the level provided is within the bounds of available levels.
func levelCheck(level Level) Level {
	switch {
	case level <= LevelTrace:
		return LevelTrace
	case level >= LevelFatal:
		return LevelFatal
	default:
		return level
	}
}

// SetLogThreshold establishes a threshold where anything matching or above will be logged.
func SetLogThreshold(level Level) {
	thresholdChanged := level != logThreshold
	if thresholdChanged {
		logThreshold = levelCheck(level)
		initialize()
	}
}

// SetStdoutThreshold establishes a threshold where anything matching or above will be output.
func SetStdoutThreshold(level Level) {
	thresholdChanged := level != outputThreshold
	if thresholdChanged {
		outputThreshold = levelCheck(level)
		initialize()
	}
}

// SetLogFile sets the LogHandle to a io.writer created for the file behind the given file path.
// Will append to this file.
func SetLogFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	LogHandle = file
	initialize()
	return nil
}

// SetLogFlag sets global log flag used in package.
func SetLogFlag(flag int) {
	flagChanged := flag != Flag
	Flag = flag
	if flagChanged {
		initialize()
	}
}
