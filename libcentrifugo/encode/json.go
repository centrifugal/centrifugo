package encode

import (
	"unicode/utf8"

	"github.com/mailru/easyjson/buffer"
)

var hex = "0123456789abcdef"

// EncodeJSONString escapes string value when encoding it to JSON.
// From https://golang.org/src/encoding/json/encode.go
func EncodeJSONString(buf *buffer.Buffer, s string, escapeHTML bool) {
	buf.AppendByte('"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if 0x20 <= b && b != '\\' && b != '"' &&
				(!escapeHTML || b != '<' && b != '>' && b != '&') {
				i++
				continue
			}
			if start < i {
				buf.AppendString(s[start:i])
			}
			switch b {
			case '\\', '"':
				buf.AppendByte('\\')
				buf.AppendByte(b)
			case '\n':
				buf.AppendByte('\\')
				buf.AppendByte('n')
			case '\r':
				buf.AppendByte('\\')
				buf.AppendByte('r')
			case '\t':
				buf.AppendByte('\\')
				buf.AppendByte('t')
			default:
				// This encodes bytes < 0x20 except for \t, \n and \r.
				// If escapeHTML is set, it also escapes <, >, and &
				// because they can lead to security holes when
				// user-controlled strings are rendered into JSON
				// and served to some browsers.
				buf.AppendString(`\u00`)
				buf.AppendByte(hex[b>>4])
				buf.AppendByte(hex[b&0xF])
			}
			i++
			start = i
			continue
		}
		c, size := utf8.DecodeRuneInString(s[i:])
		if c == utf8.RuneError && size == 1 {
			if start < i {
				buf.AppendString(s[start:i])
			}
			buf.AppendString(`\ufffd`)
			i += size
			start = i
			continue
		}
		// U+2028 is LINE SEPARATOR.
		// U+2029 is PARAGRAPH SEPARATOR.
		// They are both technically valid characters in JSON strings,
		// but don't work in JSONP, which has to be evaluated as JavaScript,
		// and can lead to security holes there. It is valid JSON to
		// escape them, so we do so unconditionally.
		// See http://timelessrepo.com/json-isnt-a-javascript-subset for discussion.
		if c == '\u2028' || c == '\u2029' {
			if start < i {
				buf.AppendString(s[start:i])
			}
			buf.AppendString(`\u202`)
			buf.AppendByte(hex[c&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	if start < len(s) {
		buf.AppendString(s[start:])
	}
	buf.AppendByte('"')
}
