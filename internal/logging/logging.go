package logging

import (
	"os"
	"runtime"
	"strings"

	"github.com/centrifugal/centrifugo/v5/internal/config"
	"github.com/centrifugal/centrifugo/v5/internal/logutils"

	"github.com/centrifugal/centrifuge"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var logLevelMatches = map[string]zerolog.Level{
	"NONE":  zerolog.NoLevel,
	"TRACE": zerolog.TraceLevel,
	"DEBUG": zerolog.DebugLevel,
	"INFO":  zerolog.InfoLevel,
	"WARN":  zerolog.WarnLevel,
	"ERROR": zerolog.ErrorLevel,
	"FATAL": zerolog.FatalLevel,
}

func configureConsoleWriter() {
	if isTerminalAttached() {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:                 os.Stdout,
			TimeFormat:          "2006-01-02 15:04:05",
			FormatLevel:         logutils.ConsoleFormatLevel(),
			FormatErrFieldName:  logutils.ConsoleFormatErrFieldName(),
			FormatErrFieldValue: logutils.ConsoleFormatErrFieldValue(),
		})
	}
}

func isTerminalAttached() bool {
	//goland:noinspection GoBoolExpressions – Goland is not smart enough here.
	return isatty.IsTerminal(os.Stdout.Fd()) && runtime.GOOS != "windows"
}

func Setup(cfg config.Config) *os.File {
	configureConsoleWriter()
	logLevel, ok := logLevelMatches[strings.ToUpper(cfg.LogLevel)]
	if !ok {
		logLevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(logLevel)
	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal().Msgf("error opening log file: %v", err)
		}
		log.Logger = log.Output(f)
		return f
	}
	return nil
}

func CentrifugeLogLevel(level string) centrifuge.LogLevel {
	if l, ok := logStringToLevel[strings.ToLower(level)]; ok {
		return l
	}
	return centrifuge.LogLevelInfo
}

// LogStringToLevel matches level string to Centrifuge LogLevel.
var logStringToLevel = map[string]centrifuge.LogLevel{
	"trace": centrifuge.LogLevelTrace,
	"debug": centrifuge.LogLevelDebug,
	"info":  centrifuge.LogLevelInfo,
	"error": centrifuge.LogLevelError,
	"none":  centrifuge.LogLevelNone,
}

type LogHandler struct {
	entries chan centrifuge.LogEntry
}

func NewCentrifugeLogHandler() *LogHandler {
	h := &LogHandler{
		entries: make(chan centrifuge.LogEntry, 64),
	}
	go h.readEntries()
	return h
}

func (h *LogHandler) readEntries() {
	for entry := range h.entries {
		var l *zerolog.Event
		switch entry.Level {
		case centrifuge.LogLevelTrace:
			l = log.Trace()
		case centrifuge.LogLevelDebug:
			l = log.Debug()
		case centrifuge.LogLevelInfo:
			l = log.Info()
		case centrifuge.LogLevelWarn:
			l = log.Warn()
		case centrifuge.LogLevelError:
			l = log.Error()
		default:
			continue
		}
		if entry.Fields != nil {
			l.Fields(entry.Fields).Msg(entry.Message)
		} else {
			l.Msg(entry.Message)
		}
	}
}

func (h *LogHandler) Handle(entry centrifuge.LogEntry) {
	select {
	case h.entries <- entry:
	default:
		return
	}
}