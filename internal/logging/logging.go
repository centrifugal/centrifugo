package logging

import (
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/logutils"

	"github.com/centrifugal/centrifuge"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Level = zerolog.Level

const (
	TraceLevel = zerolog.TraceLevel
	DebugLevel = zerolog.DebugLevel
	InfoLevel  = zerolog.InfoLevel
	WarnLevel  = zerolog.WarnLevel
	ErrorLevel = zerolog.ErrorLevel
)

var logLevelMatches = map[string]zerolog.Level{
	"NONE":  zerolog.Disabled,
	"TRACE": zerolog.TraceLevel,
	"DEBUG": zerolog.DebugLevel,
	"INFO":  zerolog.InfoLevel,
	"WARN":  zerolog.WarnLevel,
	"ERROR": zerolog.ErrorLevel,
	"FATAL": zerolog.FatalLevel,
}

func CentrifugeLogLevel(level string) centrifuge.LogLevel {
	if l, ok := logStringToLevel[strings.ToLower(level)]; ok {
		return l
	}
	return centrifuge.LogLevelInfo
}

// logStringToLevel matches level string to Centrifuge LogLevel.
var logStringToLevel = map[string]centrifuge.LogLevel{
	"trace": centrifuge.LogLevelTrace,
	"debug": centrifuge.LogLevelDebug,
	"info":  centrifuge.LogLevelInfo,
	"error": centrifuge.LogLevelError,
	"none":  centrifuge.LogLevelNone,
}

// centrifugeLogHandlerBufferSize is the number of log entries buffered between
// the (many) goroutines producing them and the single goroutine writing them
// out. Handle drops entries when this is full, so the buffer has to absorb
// bursts rather than merely smooth a steady rate.
//
// Bursts are exactly when the logs matter. A Redis failover, for instance, makes
// every shard's client and control PUB/SUB loop log connection errors, subscribe
// errors and lifecycle transitions at nearly the same instant, while the drain
// goroutine is rate-limited by writing to stdout. The previous value of 64 was
// overrun easily in that situation — and with log_level debug, which is what an
// operator turns on to investigate, it fills faster still.
const centrifugeLogHandlerBufferSize = 1024

type centrifugeLogHandler struct {
	// entries is never closed: Handle may still be called by goroutines
	// finishing after the handler was closed, and sending on a closed channel
	// panics. Entries sent after close are dropped once the buffer is full.
	entries chan centrifuge.LogEntry
	quit    chan struct{}
	done    chan struct{}
}

func newCentrifugeLogHandler() *centrifugeLogHandler {
	h := &centrifugeLogHandler{
		entries: make(chan centrifuge.LogEntry, centrifugeLogHandlerBufferSize),
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go h.readEntries()
	return h
}

func (h *centrifugeLogHandler) readEntries() {
	defer close(h.done)
	for {
		select {
		case entry := <-h.entries:
			writeEntry(entry)
		case <-h.quit:
			// Write what is buffered, then stop.
			for {
				select {
				case entry := <-h.entries:
					writeEntry(entry)
				default:
					return
				}
			}
		}
	}
}

// close stops the goroutine writing entries after it wrote the buffered ones.
func (h *centrifugeLogHandler) close() {
	close(h.quit)
	<-h.done
}

func writeEntry(entry centrifuge.LogEntry) {
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
		return
	}
	if entry.Fields != nil {
		if entry.Error != nil {
			l = l.Err(entry.Error)
			delete(entry.Fields, "error")
		}
		l.Fields(entry.Fields).Msg(entry.Message)
	} else {
		if entry.Error != nil {
			l = l.Err(entry.Error)
			delete(entry.Fields, "error")
		}
		l.Msg(entry.Message)
	}
}

func (h *centrifugeLogHandler) Handle(entry centrifuge.LogEntry) {
	select {
	case h.entries <- entry:
	default:
		return
	}
}

func configureConsoleWriter() *zerolog.ConsoleWriter {
	if isTerminalAttached() {
		return &zerolog.ConsoleWriter{
			Out:                 os.Stdout,
			TimeFormat:          "2006-01-02 15:04:05",
			FormatLevel:         logutils.ConsoleFormatLevel(),
			FormatErrFieldName:  logutils.ConsoleFormatErrFieldName(),
			FormatErrFieldValue: logutils.ConsoleFormatErrFieldValue(),
		}
	}
	return nil
}

func isTerminalAttached() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) && runtime.GOOS != "windows"
}

// Options are additional logging settings not mapped from config.
type Options struct {
	// Writers receive every log line in addition to the configured output.
	Writers []io.Writer
}

// Setup configures the global logger from config and returns a handler for
// Centrifuge logs and a function which stops the handler (after writing the
// entries it buffered), closes the log file and restores the global logger
// and level as they were before Setup. It returns an error when the
// configured log file can not be opened.
func Setup(cfg config.Config, opts Options) (centrifuge.LogHandler, func(), error) {
	var writers []io.Writer

	var file *os.File
	if cfg.Log.File != "" {
		var err error
		file, err = os.OpenFile(cfg.Log.File, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644) //nolint:gosec // Common for logging.
		if err != nil {
			return nil, nil, err
		}
		writers = append(writers, file)
	} else {
		consoleWriter := configureConsoleWriter()
		if consoleWriter != nil {
			writers = append(writers, consoleWriter)
		} else {
			writers = append(writers, os.Stderr)
		}
	}
	writers = append(writers, opts.Writers...)

	prevOut := globalOut.get()
	prevLevel := zerolog.GlobalLevel()

	levelFound := true
	logLevel, ok := logLevelMatches[strings.ToUpper(cfg.Log.Level)]
	if !ok {
		levelFound = false
		logLevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(logLevel)

	if len(writers) > 0 {
		globalOut.set(io.MultiWriter(writers...))
		installGlobalOut()
	}

	if !levelFound {
		log.Warn().Msgf("unknown log level %s, defaulting to INFO", cfg.Log.Level)
	}

	handler := newCentrifugeLogHandler()
	var closeOnce sync.Once
	return handler.Handle, func() {
		closeOnce.Do(func() {
			handler.close()
			globalOut.set(prevOut)
			zerolog.SetGlobalLevel(prevLevel)
			if file != nil {
				_ = file.Close()
			}
		})
	}, nil
}

// Enabled checks if a specific logging level is enabled
func Enabled(level Level) bool {
	return level >= zerolog.GlobalLevel()
}

// The global logger writes through globalOut, installed once: a later Setup
// (another app in the same process, as tests may run several) or a close swaps
// the writer, and never reassigns log.Logger while goroutines log.
var (
	globalOut     = &swapWriter{}
	installGlobal sync.Once
)

func installGlobalOut() {
	installGlobal.Do(func() { log.Logger = log.Output(globalOut) })
}

type swapWriter struct{ w atomic.Pointer[io.Writer] }

func (s *swapWriter) get() io.Writer {
	if p := s.w.Load(); p != nil {
		return *p
	}
	return nil
}

func (s *swapWriter) set(w io.Writer) {
	if w == nil {
		w = os.Stderr
	}
	s.w.Store(&w)
}

func (s *swapWriter) Write(p []byte) (int, error) {
	w := s.get()
	if w == nil {
		w = os.Stderr
	}
	return w.Write(p)
}
