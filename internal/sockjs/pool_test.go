package sockjs

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringReaderPool(t *testing.T) {
	r := GetStringReader("string1")
	d, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, []byte("string1"), d)
	PutStringReader(r)
	r = GetStringReader("string2")
	defer PutStringReader(r)
	d, err = io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, []byte("string2"), d)
}

func TestBytesReaderPool(t *testing.T) {
	r := GetBytesReader([]byte("bytes1"))
	d, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, []byte("bytes1"), d)
	PutBytesReader(r)
	r = GetBytesReader([]byte("bytes2"))
	defer PutBytesReader(r)
	d, err = io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, []byte("bytes2"), d)
}
