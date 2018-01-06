// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package argon2

import (
	"bytes"
	"encoding/hex"
	"testing"
)

var (
	genKatPassword = []byte{
		0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
		0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
		0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
		0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
	}
	genKatSalt   = []byte{0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02}
	genKatSecret = []byte{0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03}
	genKatAAD    = []byte{0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04}
)

func TestArgon2(t *testing.T) {
	defer func(sse4 bool) { useSSE4 = sse4 }(useSSE4)

	if useSSE4 {
		t.Log("SSE4.1 version")
		testArgon2i(t)
		testArgon2d(t)
		testArgon2id(t)
		useSSE4 = false
	}
	t.Log("generic version")
	testArgon2i(t)
	testArgon2d(t)
	testArgon2id(t)
}

func testArgon2d(t *testing.T) {
	want := []byte{
		0x51, 0x2b, 0x39, 0x1b, 0x6f, 0x11, 0x62, 0x97,
		0x53, 0x71, 0xd3, 0x09, 0x19, 0x73, 0x42, 0x94,
		0xf8, 0x68, 0xe3, 0xbe, 0x39, 0x84, 0xf3, 0xc1,
		0xa1, 0x3a, 0x4d, 0xb9, 0xfa, 0xbe, 0x4a, 0xcb,
	}
	hash := deriveKey(argon2d, genKatPassword, genKatSalt, genKatSecret, genKatAAD, 3, 32, 4, 32)
	if !bytes.Equal(hash, want) {
		t.Errorf("derived key does not match - got: %s , want: %s", hex.EncodeToString(hash), hex.EncodeToString(want))
	}
}

func testArgon2i(t *testing.T) {
	want := []byte{
		0xc8, 0x14, 0xd9, 0xd1, 0xdc, 0x7f, 0x37, 0xaa,
		0x13, 0xf0, 0xd7, 0x7f, 0x24, 0x94, 0xbd, 0xa1,
		0xc8, 0xde, 0x6b, 0x01, 0x6d, 0xd3, 0x88, 0xd2,
		0x99, 0x52, 0xa4, 0xc4, 0x67, 0x2b, 0x6c, 0xe8,
	}
	hash := deriveKey(argon2i, genKatPassword, genKatSalt, genKatSecret, genKatAAD, 3, 32, 4, 32)
	if !bytes.Equal(hash, want) {
		t.Errorf("derived key does not match - got: %s , want: %s", hex.EncodeToString(hash), hex.EncodeToString(want))
	}
}

func testArgon2id(t *testing.T) {
	want := []byte{
		0x0d, 0x64, 0x0d, 0xf5, 0x8d, 0x78, 0x76, 0x6c,
		0x08, 0xc0, 0x37, 0xa3, 0x4a, 0x8b, 0x53, 0xc9,
		0xd0, 0x1e, 0xf0, 0x45, 0x2d, 0x75, 0xb6, 0x5e,
		0xb5, 0x25, 0x20, 0xe9, 0x6b, 0x01, 0xe6, 0x59,
	}
	hash := deriveKey(argon2id, genKatPassword, genKatSalt, genKatSecret, genKatAAD, 3, 32, 4, 32)
	if !bytes.Equal(hash, want) {
		t.Errorf("derived key does not match - got: %s , want: %s", hex.EncodeToString(hash), hex.EncodeToString(want))
	}
}

func benchmarkArgon2(mode int, time, memory uint32, threads uint8, keyLen uint32, b *testing.B) {
	password := []byte("password")
	salt := []byte("choosing random salts is hard")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		deriveKey(mode, password, salt, nil, nil, time, memory, threads, keyLen)
	}
}

func BenchmarkArgon2i(b *testing.B) {
	b.Run(" Time: 3 Memory: 32 MB, Threads: 1", func(b *testing.B) { benchmarkArgon2(argon2i, 3, 32*1024, 1, 32, b) })
	b.Run(" Time: 4 Memory: 32 MB, Threads: 1", func(b *testing.B) { benchmarkArgon2(argon2i, 4, 32*1024, 1, 32, b) })
	b.Run(" Time: 5 Memory: 32 MB, Threads: 1", func(b *testing.B) { benchmarkArgon2(argon2i, 5, 32*1024, 1, 32, b) })
	b.Run(" Time: 3 Memory: 64 MB, Threads: 4", func(b *testing.B) { benchmarkArgon2(argon2i, 3, 64*1024, 4, 32, b) })
	b.Run(" Time: 4 Memory: 64 MB, Threads: 4", func(b *testing.B) { benchmarkArgon2(argon2i, 4, 64*1024, 4, 32, b) })
	b.Run(" Time: 5 Memory: 64 MB, Threads: 4", func(b *testing.B) { benchmarkArgon2(argon2i, 5, 64*1024, 4, 32, b) })
}

func BenchmarkArgon2d(b *testing.B) {
	b.Run(" Time: 3, Memory: 32 MB, Threads: 1", func(b *testing.B) { benchmarkArgon2(argon2d, 3, 32*1024, 1, 32, b) })
	b.Run(" Time: 4, Memory: 32 MB, Threads: 1", func(b *testing.B) { benchmarkArgon2(argon2d, 4, 32*1024, 1, 32, b) })
	b.Run(" Time: 5, Memory: 32 MB, Threads: 1", func(b *testing.B) { benchmarkArgon2(argon2d, 5, 32*1024, 1, 32, b) })
	b.Run(" Time: 3, Memory: 64 MB, Threads: 4", func(b *testing.B) { benchmarkArgon2(argon2d, 3, 64*1024, 4, 32, b) })
	b.Run(" Time: 4, Memory: 64 MB, Threads: 4", func(b *testing.B) { benchmarkArgon2(argon2d, 4, 64*1024, 4, 32, b) })
	b.Run(" Time: 5, Memory: 64 MB, Threads: 4", func(b *testing.B) { benchmarkArgon2(argon2d, 5, 64*1024, 4, 32, b) })
}

func BenchmarkArgon2id(b *testing.B) {
	b.Run(" Time: 3, Memory: 32 MB, Threads: 1", func(b *testing.B) { benchmarkArgon2(argon2id, 3, 32*1024, 1, 32, b) })
	b.Run(" Time: 4, Memory: 32 MB, Threads: 1", func(b *testing.B) { benchmarkArgon2(argon2id, 4, 32*1024, 1, 32, b) })
	b.Run(" Time: 5, Memory: 32 MB, Threads: 1", func(b *testing.B) { benchmarkArgon2(argon2id, 5, 32*1024, 1, 32, b) })
	b.Run(" Time: 3, Memory: 64 MB, Threads: 4", func(b *testing.B) { benchmarkArgon2(argon2id, 3, 64*1024, 4, 32, b) })
	b.Run(" Time: 4, Memory: 64 MB, Threads: 4", func(b *testing.B) { benchmarkArgon2(argon2id, 4, 64*1024, 4, 32, b) })
	b.Run(" Time: 5, Memory: 64 MB, Threads: 4", func(b *testing.B) { benchmarkArgon2(argon2id, 5, 64*1024, 4, 32, b) })
}
