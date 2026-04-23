//go:build !appengine

package pgstreambroker

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/centrifugal/centrifugo/v6/internal/convert"
)

// byteArena is a chunked arena allocator for byte data. It copies incoming
// slices into a contiguous buffer and hands back sub-slices (or strings via
// unsafe conversion). When the current chunk is full a new one is allocated;
// old chunks stay alive via the slices/strings already referencing them.
//
// Copied from internal/pgmapbroker/pgx_helpers.go. Both packages need this
// for zero-allocation row scanning. A future refactor could extract these
// helpers into a shared internal package; for now they're duplicated to
// keep the packages independent.
type byteArena struct {
	buf []byte
}

// copyBytes copies src into the arena and returns a sub-slice of the arena buffer.
func (a *byteArena) copyBytes(src []byte) []byte {
	n := len(src)
	if n == 0 {
		return nil
	}
	if cap(a.buf)-len(a.buf) < n {
		newCap := 2 * cap(a.buf)
		if newCap < n {
			newCap = n
		}
		if newCap < 4096 {
			newCap = 4096
		}
		a.buf = make([]byte, 0, newCap)
	}
	start := len(a.buf)
	a.buf = append(a.buf, src...)
	return a.buf[start : start+n : start+n]
}

// copyString copies src into the arena and returns a string backed by arena memory.
func (a *byteArena) copyString(src []byte) string {
	if len(src) == 0 {
		return ""
	}
	b := a.copyBytes(src)
	return convert.BytesToString(b)
}

// pgTextFormat is the pgx text wire format code.
const pgTextFormat int16 = 0

// pgRawInt64 parses a pgx int8 value as int64.
func pgRawInt64(b []byte, format int16) int64 {
	if b == nil {
		return 0
	}
	if format == pgTextFormat {
		v, _ := strconv.ParseInt(convert.BytesToString(b), 10, 64)
		return v
	}
	return int64(binary.BigEndian.Uint64(b))
}

// pgRawUint64 parses a pgx int8 value as uint64.
func pgRawUint64(b []byte, format int16) uint64 {
	if b == nil {
		return 0
	}
	if format == pgTextFormat {
		v, _ := strconv.ParseUint(convert.BytesToString(b), 10, 64)
		return v
	}
	return binary.BigEndian.Uint64(b)
}

// pgRawInt16 parses a pgx int2 (SMALLINT) value as int16.
// Handles both binary wire format (2-byte big-endian) and text (decimal string).
func pgRawInt16(b []byte, format int16) int16 {
	if b == nil {
		return 0
	}
	if format == pgTextFormat {
		v, _ := strconv.ParseInt(convert.BytesToString(b), 10, 16)
		return int16(v)
	}
	return int16(binary.BigEndian.Uint16(b))
}

// pgRawString copies a pgx text value into the arena and returns the resulting
// string. TEXT/VARCHAR have identical representation in both binary and text wire
// formats, so no format parameter is needed. Returns "" for nil input.
func pgRawString(a *byteArena, b []byte) string {
	if b == nil {
		return ""
	}
	return a.copyString(b)
}

// pgRawBytes copies a pgx BYTEA value into the arena.
func pgRawBytes(a *byteArena, b []byte, format int16) []byte {
	if b == nil {
		return nil
	}
	if format == pgTextFormat && len(b) >= 2 && b[0] == '\\' && b[1] == 'x' {
		decoded := make([]byte, hex.DecodedLen(len(b)-2))
		n, err := hex.Decode(decoded, b[2:])
		if err != nil {
			return a.copyBytes(b) // fallback: copy as-is
		}
		return a.copyBytes(decoded[:n])
	}
	return a.copyBytes(b)
}

// pgRawJSONBBytes extracts the JSON body from a pgx JSONB value into the arena.
func pgRawJSONBBytes(a *byteArena, b []byte, format int16) []byte {
	if b == nil {
		return nil
	}
	if format != pgTextFormat && len(b) > 1 {
		return a.copyBytes(b[1:]) // strip version byte
	}
	return a.copyBytes(b)
}

// pgColFormats holds per-column wire format codes extracted from pgx rows.
type pgColFormats []int16

// pgRawJSONBMap parses a JSONB value into a map[string]string.
func pgRawJSONBMap(b []byte) map[string]string {
	if len(b) == 0 {
		return nil
	}
	data := b
	if len(b) > 1 && b[0] != '{' && b[0] != '[' && b[0] != 'n' {
		data = b[1:] // skip version byte
	}
	var m map[string]string
	_ = json.Unmarshal(data, &m)
	return m
}
