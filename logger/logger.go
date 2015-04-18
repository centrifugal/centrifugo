// This is an adapted code from Steve Francia's jWalterWeatherman
// library - see https://github.com/spf13/jWalterWeatherman
package logger

import (
	"io"
	"io/ioutil"
	"log"
	"os"
)

// Level describes the chosen log level
type Level int

type NotePad struct {
	Handle io.Writer
	Level  Level
	Prefix string
	Logger **log.Logger
}

const (
	LevelDebug Level = iota
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
	DEBUG    *log.Logger
	INFO     *log.Logger
	WARN     *log.Logger
	ERROR    *log.Logger
	CRITICAL *log.Logger
	FATAL    *log.Logger

	LogHandle  io.Writer = ioutil.Discard
	OutHandle  io.Writer = os.Stdout
	BothHandle io.Writer = io.MultiWriter(LogHandle, OutHandle)

	Flag int = log.Ldate | log.Ltime | log.Lshortfile

	NotePads []*NotePad = []*NotePad{debug, info, warn, err, critical, fatal}

	debug    *NotePad = &NotePad{Level: LevelDebug, Handle: os.Stdout, Logger: &DEBUG, Prefix: "DEBUG: "}
	info     *NotePad = &NotePad{Level: LevelInfo, Handle: os.Stdout, Logger: &INFO, Prefix: "INFO: "}
	warn     *NotePad = &NotePad{Level: LevelWarn, Handle: os.Stdout, Logger: &WARN, Prefix: "WARN: "}
	err      *NotePad = &NotePad{Level: LevelError, Handle: os.Stdout, Logger: &ERROR, Prefix: "ERROR: "}
	critical *NotePad = &NotePad{Level: LevelCritical, Handle: os.Stdout, Logger: &CRITICAL, Prefix: "CRITICAL: "}
	fatal    *NotePad = &NotePad{Level: LevelFatal, Handle: os.Stdout, Logger: &FATAL, Prefix: "FATAL: "}

	logThreshold    Level = DefaultLogThreshold
	outputThreshold Level = DefaultStdoutThreshold
)

var LevelMatches = map[string]Level{
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

// initialize initializes loggers
func initialize() {
	BothHandle = io.MultiWriter(LogHandle, OutHandle)

	for _, n := range NotePads {
		if n.Level < outputThreshold && n.Level < logThreshold {
			n.Handle = ioutil.Discard
		} else if n.Level >= outputThreshold && n.Level >= logThreshold {
			n.Handle = BothHandle
		} else if n.Level >= outputThreshold && n.Level < logThreshold {
			n.Handle = OutHandle
		} else {
			n.Handle = LogHandle
		}
	}

	for _, n := range NotePads {
		*n.Logger = log.New(n.Handle, n.Prefix, Flag)
	}
}

// Ensures that the level provided is within the bounds of available levels
func levelCheck(level Level) Level {
	switch {
	case level <= LevelDebug:
		return LevelDebug
	case level >= LevelFatal:
		return LevelFatal
	default:
		return level
	}
}

// Establishes a threshold where anything matching or above will be logged
func SetLogThreshold(level Level) {
	logThreshold = levelCheck(level)
	initialize()
}

// Establishes a threshold where anything matching or above will be output
func SetStdoutThreshold(level Level) {
	outputThreshold = levelCheck(level)
	initialize()
}

// Conveniently Sets the Log Handle to a io.writer created for the file behind the given filepath
// Will only append to this file
func SetLogFile(path string) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		CRITICAL.Println("Failed to open log file:", path, err)
		os.Exit(-1)
	}

	LogHandle = file
	initialize()
}

func SetLogFlag(flag int) {
	Flag = flag
	initialize()
}
