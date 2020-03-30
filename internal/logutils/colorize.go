package logutils

import (
	"fmt"

	"github.com/rs/zerolog"
)

const (
	colorBlack = iota + 30
	colorRed
	colorGreen
	colorYellow
	colorBlue
	colorMagenta
	colorCyan

	colorBold = 1
)

// colorize returns the string s wrapped in ANSI code c, unless disabled is true.
func colorize(s interface{}, c int) string {
	return fmt.Sprintf("\x1b[%dm%v\x1b[0m", c, s)
}

// ConsoleFormatLevel returns a custom colorizer for zerolog console level output.
func ConsoleFormatLevel() zerolog.Formatter {
	return func(i interface{}) string {
		var l string
		if ll, ok := i.(string); ok {
			switch ll {
			case "debug":
				l = colorize("DBG", colorMagenta)
			case "info":
				l = colorize("INF", colorGreen)
			case "warn":
				l = colorize("WRN", colorYellow)
			case "error":
				l = colorize("ERR", colorRed)
			case "fatal":
				l = colorize(colorize("FTL", colorRed), colorBold)
			default:
				l = colorize("???", colorBold)
			}
		} else {
			l = colorize("???", colorBold)
		}
		return l
	}
}

// ConsoleFormatErrFieldName returns custom formatter for error field name.
func ConsoleFormatErrFieldName() zerolog.Formatter {
	return func(i interface{}) string {
		return fmt.Sprintf("%s=", i)
	}
}

// ConsoleFormatErrFieldValue returns custom formatter for error value.
func ConsoleFormatErrFieldValue() zerolog.Formatter {
	return func(i interface{}) string {
		return fmt.Sprintf("%s", i)
	}
}
