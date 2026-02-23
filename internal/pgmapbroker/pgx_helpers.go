//go:build !appengine

package pgmapbroker

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"
	"unsafe"
)

// byteArena is a chunked arena allocator for byte data. It copies incoming
// slices into a contiguous buffer and hands back sub-slices (or strings via
// unsafe conversion). When the current chunk is full a new one is allocated;
// old chunks stay alive via the slices/strings already referencing them.
type byteArena struct {
	buf []byte
}

// copyBytes copies src into the arena and returns a sub-slice of the arena buffer.
// The returned slice has cap==len (three-index slice) so that append on it
// cannot silently overwrite adjacent arena data.
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
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// pgTextFormat is the pgx text wire format code.
const pgTextFormat int16 = 0

// pgRawInt64 parses a pgx int8 value as int64.
// Handles both binary wire format (8-byte big-endian) and text (decimal string).
// Returns 0 for nil input.
func pgRawInt64(b []byte, format int16) int64 {
	if b == nil {
		return 0
	}
	if format == pgTextFormat {
		v, _ := strconv.ParseInt(unsafe.String(unsafe.SliceData(b), len(b)), 10, 64)
		return v
	}
	return int64(binary.BigEndian.Uint64(b))
}

// pgRawUint64 parses a pgx int8 value as uint64.
// Handles both binary wire format (8-byte big-endian) and text (decimal string).
// Returns 0 for nil input.
func pgRawUint64(b []byte, format int16) uint64 {
	if b == nil {
		return 0
	}
	if format == pgTextFormat {
		v, _ := strconv.ParseUint(unsafe.String(unsafe.SliceData(b), len(b)), 10, 64)
		return v
	}
	return binary.BigEndian.Uint64(b)
}

// pgRawInt32 parses a pgx int4 value as int32.
// Handles both binary wire format (4-byte big-endian) and text (decimal string).
// Returns 0 for nil input.
func pgRawInt32(b []byte, format int16) int32 {
	if b == nil {
		return 0
	}
	if format == pgTextFormat {
		v, _ := strconv.ParseInt(unsafe.String(unsafe.SliceData(b), len(b)), 10, 32)
		return int32(v)
	}
	return int32(binary.BigEndian.Uint32(b))
}

// pgRawBool parses a pgx boolean value.
// Handles both binary wire format (0x00/0x01) and text ("t"/"f").
// Returns false for nil input.
func pgRawBool(b []byte, format int16) bool {
	if b == nil {
		return false
	}
	if format == pgTextFormat {
		return len(b) > 0 && b[0] == 't'
	}
	return b[0] == 1
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
// Handles both binary wire format (raw bytes) and text (hex-encoded \xDEAD...).
// Returns nil for nil input.
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
// Binary wire format has a 1-byte version header that must be stripped.
// Text wire format is plain JSON (no header). Returns nil for nil input.
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
// Format values: 0 = text, 1 = binary. Call pgColFormatsFromRows once per
// query result, before iterating rows.
type pgColFormats []int16

// pgEpochDeltaUsec is the difference in microseconds between the Unix epoch
// (1970-01-01) and the PostgreSQL epoch (2000-01-01).
const pgEpochDeltaUsec = 946684800_000_000

// pgRawTimestampMillis parses a pgx TIMESTAMPTZ value as Unix milliseconds.
// Binary wire format: 8-byte big-endian int64 = microseconds since PG epoch (2000-01-01 UTC).
// Text wire format: parsed with time.Parse. Returns 0 for nil input.
func pgRawTimestampMillis(b []byte, format int16) int64 {
	if b == nil {
		return 0
	}
	if format == pgTextFormat {
		// PostgreSQL text format for timestamptz, e.g. "2025-06-15 12:34:56.123456+00"
		t, err := time.Parse("2006-01-02 15:04:05.999999Z07:00:00", unsafe.String(unsafe.SliceData(b), len(b)))
		if err != nil {
			return 0
		}
		return t.UnixMilli()
	}
	usec := int64(binary.BigEndian.Uint64(b))
	return (usec + pgEpochDeltaUsec) / 1000
}

// pgRawJSONBMap parses a JSONB value into a map[string]string.
// Handles both binary wire format (1-byte version header + JSON body)
// and text wire format (plain JSON). Returns nil for nil or empty input.
func pgRawJSONBMap(b []byte) map[string]string {
	if len(b) == 0 {
		return nil
	}
	// Binary format has a version prefix byte (typically 1).
	// Text format starts directly with '{'.
	data := b
	if len(b) > 1 && b[0] != '{' && b[0] != '[' && b[0] != 'n' {
		data = b[1:] // skip version byte
	}
	var m map[string]string
	_ = json.Unmarshal(data, &m)
	return m
}
