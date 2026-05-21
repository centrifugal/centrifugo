package pgmapbroker

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	fmtText   int16 = 0
	fmtBinary int16 = 1
)

func TestByteArena_CopyBytes(t *testing.T) {
	a := byteArena{}

	t.Run("nil_input", func(t *testing.T) {
		require.Nil(t, a.copyBytes(nil))
	})
	t.Run("empty_input", func(t *testing.T) {
		require.Nil(t, a.copyBytes([]byte{}))
	})
	t.Run("copy_is_independent", func(t *testing.T) {
		src := []byte("hello")
		got := a.copyBytes(src)
		require.Equal(t, []byte("hello"), got)
		// Mutating source must not affect copy.
		src[0] = 'H'
		require.Equal(t, byte('h'), got[0])
	})
	t.Run("cap_equals_len", func(t *testing.T) {
		got := a.copyBytes([]byte("abc"))
		require.Equal(t, len(got), cap(got))
	})
	t.Run("growth", func(t *testing.T) {
		a2 := byteArena{}
		// Fill beyond initial chunk.
		big := make([]byte, 5000)
		for i := range big {
			big[i] = byte(i % 256)
		}
		got := a2.copyBytes(big)
		require.Equal(t, big, got)
		require.Equal(t, len(got), cap(got))
	})
}

func TestByteArena_CopyString(t *testing.T) {
	a := byteArena{}

	t.Run("nil_input", func(t *testing.T) {
		require.Equal(t, "", a.copyString(nil))
	})
	t.Run("empty_input", func(t *testing.T) {
		require.Equal(t, "", a.copyString([]byte{}))
	})
	t.Run("copies_content", func(t *testing.T) {
		src := []byte("world")
		got := a.copyString(src)
		require.Equal(t, "world", got)
		src[0] = 'W'
		require.Equal(t, "world", got)
	})
}

func putUint64(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func putUint32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

func TestPgRawInt64(t *testing.T) {
	// Binary format.
	require.Equal(t, int64(0), pgRawInt64(nil, fmtBinary))
	require.Equal(t, int64(0), pgRawInt64(putUint64(0), fmtBinary))
	require.Equal(t, int64(42), pgRawInt64(putUint64(42), fmtBinary))
	require.Equal(t, int64(-1), pgRawInt64(putUint64(math.MaxUint64), fmtBinary))
	require.Equal(t, int64(math.MinInt64), pgRawInt64(putUint64(1<<63), fmtBinary))
	// Text format.
	require.Equal(t, int64(0), pgRawInt64(nil, fmtText))
	require.Equal(t, int64(0), pgRawInt64([]byte("0"), fmtText))
	require.Equal(t, int64(42), pgRawInt64([]byte("42"), fmtText))
	require.Equal(t, int64(-1), pgRawInt64([]byte("-1"), fmtText))
	require.Equal(t, int64(12345678), pgRawInt64([]byte("12345678"), fmtText))
	require.Equal(t, int64(math.MaxInt64), pgRawInt64([]byte("9223372036854775807"), fmtText))
}

func TestPgRawUint64(t *testing.T) {
	// Binary format.
	require.Equal(t, uint64(0), pgRawUint64(nil, fmtBinary))
	require.Equal(t, uint64(0), pgRawUint64(putUint64(0), fmtBinary))
	require.Equal(t, uint64(123456789), pgRawUint64(putUint64(123456789), fmtBinary))
	require.Equal(t, uint64(math.MaxUint64), pgRawUint64(putUint64(math.MaxUint64), fmtBinary))
	// Text format.
	require.Equal(t, uint64(0), pgRawUint64(nil, fmtText))
	require.Equal(t, uint64(0), pgRawUint64([]byte("0"), fmtText))
	require.Equal(t, uint64(123456789), pgRawUint64([]byte("123456789"), fmtText))
}

func TestPgRawInt32(t *testing.T) {
	// Binary format.
	require.Equal(t, int32(0), pgRawInt32(nil, fmtBinary))
	require.Equal(t, int32(0), pgRawInt32(putUint32(0), fmtBinary))
	require.Equal(t, int32(100), pgRawInt32(putUint32(100), fmtBinary))
	require.Equal(t, int32(-1), pgRawInt32(putUint32(math.MaxUint32), fmtBinary))
	// Text format.
	require.Equal(t, int32(0), pgRawInt32(nil, fmtText))
	require.Equal(t, int32(100), pgRawInt32([]byte("100"), fmtText))
	require.Equal(t, int32(-1), pgRawInt32([]byte("-1"), fmtText))
}

func TestPgRawBool(t *testing.T) {
	// Binary format.
	require.False(t, pgRawBool(nil, fmtBinary))
	require.True(t, pgRawBool([]byte{1}, fmtBinary))
	require.False(t, pgRawBool([]byte{0}, fmtBinary))
	// Text format.
	require.False(t, pgRawBool(nil, fmtText))
	require.True(t, pgRawBool([]byte("t"), fmtText))
	require.False(t, pgRawBool([]byte("f"), fmtText))
}

func TestPgRawString(t *testing.T) {
	a := byteArena{}
	require.Equal(t, "", pgRawString(&a, nil))
	require.Equal(t, "hello", pgRawString(&a, []byte("hello")))
}

func TestPgRawBytes(t *testing.T) {
	// Binary format: raw bytes.
	a := byteArena{}
	require.Nil(t, pgRawBytes(&a, nil, fmtBinary))
	require.Equal(t, []byte{0xde, 0xad}, pgRawBytes(&a, []byte{0xde, 0xad}, fmtBinary))
	// Text format: hex-encoded \xDEAD.
	a2 := byteArena{}
	require.Equal(t, []byte{0xde, 0xad}, pgRawBytes(&a2, []byte(`\xdead`), fmtText))
	// Text format: non-hex passthrough.
	a3 := byteArena{}
	require.Equal(t, []byte("hello"), pgRawBytes(&a3, []byte("hello"), fmtText))
}

func TestPgRawJSONBBytes(t *testing.T) {
	// Binary format: strip version byte.
	a := byteArena{}
	require.Nil(t, pgRawJSONBBytes(&a, nil, fmtBinary))
	b := append([]byte{1}, []byte(`{"foo":"bar"}`)...)
	require.Equal(t, []byte(`{"foo":"bar"}`), pgRawJSONBBytes(&a, b, fmtBinary))
	// Text format: plain JSON.
	a2 := byteArena{}
	require.Equal(t, []byte(`{"foo":"bar"}`), pgRawJSONBBytes(&a2, []byte(`{"foo":"bar"}`), fmtText))
}

func TestPgRawJSONBMap(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		require.Nil(t, pgRawJSONBMap(nil))
	})
	t.Run("empty", func(t *testing.T) {
		require.Nil(t, pgRawJSONBMap([]byte{}))
	})
	t.Run("binary_empty_object", func(t *testing.T) {
		// Binary wire format: version byte (1) + empty JSON object.
		b := append([]byte{1}, []byte("{}")...)
		got := pgRawJSONBMap(b)
		require.NotNil(t, got)
		require.Empty(t, got)
	})
	t.Run("binary_valid_map", func(t *testing.T) {
		// Binary wire format: version byte (1) + JSON.
		b := append([]byte{1}, []byte(`{"a":"1","b":"2"}`)...)
		got := pgRawJSONBMap(b)
		require.Equal(t, map[string]string{"a": "1", "b": "2"}, got)
	})
	t.Run("text_empty_object", func(t *testing.T) {
		// Text wire format: plain JSON (no version byte).
		got := pgRawJSONBMap([]byte("{}"))
		require.NotNil(t, got)
		require.Empty(t, got)
	})
	t.Run("text_valid_map", func(t *testing.T) {
		// Text wire format: plain JSON (no version byte).
		got := pgRawJSONBMap([]byte(`{"sector":"tech"}`))
		require.Equal(t, map[string]string{"sector": "tech"}, got)
	})
	t.Run("text_null", func(t *testing.T) {
		// Text wire format: JSON null.
		require.Nil(t, pgRawJSONBMap([]byte("null")))
	})
}
