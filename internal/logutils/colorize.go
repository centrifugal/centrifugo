package logutils

import (
	"fmt"

	"github.com/rs/zerolog"
)

const (
	colorRed = iota + 31
	colorGreen
	colorYellow
	colorBlue
	colorMagenta
	_

	colorBold = 1
)

// colorize returns the string s wrapped in ANSI code c, unless disabled is true.
func colorize(s any, c int) string {
	return fmt.Sprintf("\x1b[%dm%v\x1b[0m", c, s)
}

func wrap(s string) string {
	return fmt.Sprintf("[%s]", s)
}

var traceLabel = wrap(colorize("TRC", colorBlue))
var debugLabel = wrap(colorize("DBG", colorMagenta))
var infoLabel = wrap(colorize("INF", colorGreen))
var warnLabel = wrap(colorize("WRN", colorYellow))
var errorLabel = wrap(colorize("ERR", colorRed))
var fatalLabel = wrap(colorize(colorize("FTL", colorRed), colorBold))
var unknownLabel = wrap(colorize("???", colorRed))

// ConsoleFormatLevel returns a custom colorizer for zerolog console level output.
func ConsoleFormatLevel() zerolog.Formatter {
	return func(i any) string {
		if ll, ok := i.(string); ok {
			switch ll {
			case "trace":
				return traceLabel
			case "debug":
				return debugLabel
			case "info":
				return infoLabel
			case "warn":
				return warnLabel
			case "error":
				return errorLabel
			case "fatal":
				return fatalLabel
			default:
				return unknownLabel
			}
		}
		return unknownLabel
	}
}

// ConsoleFormatErrFieldName returns custom formatter for error field name.
func ConsoleFormatErrFieldName() zerolog.Formatter {
	return func(i any) string {
		return fmt.Sprintf("%s=", i)
	}
}

// ConsoleFormatErrFieldValue returns custom formatter for error value.
func ConsoleFormatErrFieldValue() zerolog.Formatter {
	return func(i any) string {
		return fmt.Sprintf("%s", i)
	}
}
