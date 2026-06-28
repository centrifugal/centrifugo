package websocket

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"testing"
)

// writeCompressedFrame returns a buffer holding a single permessage-deflate
// compressed binary message carrying payload. The frame is written as a client
// (masked) so it can be consumed by a server-side reader.
func writeCompressedFrame(t *testing.T, payload []byte, level int) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	wc := newTestConn(nil, &buf, false)
	wc.newCompressionWriter = compressNoContextTakeover
	if level != 0 {
		if err := wc.SetCompressionLevel(level); err != nil {
			t.Fatalf("SetCompressionLevel: %v", err)
		}
	}
	w, err := wc.NextWriter(BinaryMessage)
	if err != nil {
		t.Fatalf("NextWriter: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return &buf
}

// newCompressedReader builds a server-side reader for frames stored in src,
// with permessage-deflate decompression enabled and the given decompressed read
// limit. out captures any control frames (e.g. CloseMessageTooBig) the reader
// writes back to the peer.
func newCompressedReader(src *bytes.Buffer, decompressedLimit int64) (rc *Conn, out *bytes.Buffer) {
	out = &bytes.Buffer{}
	rc = newTestConn(src, out, true)
	rc.newDecompressionReader = decompressNoContextTakeover
	rc.SetDecompressedReadLimit(decompressedLimit)
	return rc, out
}

func sentTooBig(out *bytes.Buffer) bool {
	// FormatCloseMessage encodes the 2-byte close code; the server reader does
	// not mask control frames, so the encoded code appears verbatim in out.
	return bytes.Contains(out.Bytes(), FormatCloseMessage(CloseMessageTooBig, ""))
}

// TestDecompressedReadLimit_Bomb proves that a tiny compressed frame that
// inflates to a huge payload is rejected with ErrReadLimit, that the close
// frame is sent, and crucially that memory is bounded: io.ReadAll stops after
// at most limit+1 bytes instead of materializing the whole bomb.
func TestDecompressedReadLimit_Bomb(t *testing.T) {
	const limit = 64 * 1024
	const decompressedSize = 8 * 1024 * 1024 // 8MB inflates from a few KB.

	src := writeCompressedFrame(t, bytes.Repeat([]byte{'A'}, decompressedSize), 9)
	if src.Len() >= limit {
		t.Fatalf("compressed frame %d bytes is not under the compressed read limit %d; test would not isolate the decompressed limit", src.Len(), limit)
	}

	rc, out := newCompressedReader(src, limit)

	mt, r, err := rc.NextReader()
	if err != nil {
		t.Fatalf("NextReader: %v", err)
	}
	if mt != BinaryMessage {
		t.Fatalf("message type = %d, want %d", mt, BinaryMessage)
	}

	data, err := io.ReadAll(r)
	if !errors.Is(err, ErrReadLimit) {
		t.Fatalf("io.ReadAll error = %v, want ErrReadLimit", err)
	}
	if int64(len(data)) > limit+1 {
		t.Fatalf("read %d bytes, want at most %d (memory must stay bounded)", len(data), limit+1)
	}
	if !sentTooBig(out) {
		t.Fatalf("expected CloseMessageTooBig control frame to be sent")
	}
	// The server-sent 1009 must be observable as an outgoing close code.
	if code, incoming := rc.CloseCode(); code != CloseMessageTooBig || incoming {
		t.Fatalf("CloseCode() = (%d, %v), want (%d, false)", code, incoming, CloseMessageTooBig)
	}
}

// TestDecompressedReadLimit_Boundary proves the exact accept/reject boundary:
// a message of exactly limit bytes is accepted, limit+1 is rejected.
func TestDecompressedReadLimit_Boundary(t *testing.T) {
	const limit = 4096

	cases := []struct {
		name       string
		size       int
		wantErr    bool
		wantTooBig bool
	}{
		{"under", limit - 1, false, false},
		{"exact", limit, false, false},
		{"over_by_one", limit + 1, true, true},
		{"over_by_many", limit * 4, true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := bytes.Repeat([]byte{'x'}, tc.size)
			src := writeCompressedFrame(t, payload, 0)
			rc, out := newCompressedReader(src, limit)

			_, r, err := rc.NextReader()
			if err != nil {
				t.Fatalf("NextReader: %v", err)
			}
			data, err := io.ReadAll(r)

			if tc.wantErr {
				if !errors.Is(err, ErrReadLimit) {
					t.Fatalf("error = %v, want ErrReadLimit", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !bytes.Equal(data, payload) {
					t.Fatalf("decompressed payload mismatch: got %d bytes, want %d", len(data), len(payload))
				}
			}
			if got := sentTooBig(out); got != tc.wantTooBig {
				t.Fatalf("sentTooBig = %v, want %v", got, tc.wantTooBig)
			}
		})
	}
}

// TestDecompressedReadLimit_LegitCompressible proves the fix does not break the
// legitimate case the report worried about: a message whose compressed size is
// small but whose decompressed size is well above the compressed bytes is still
// accepted, as long as it stays within the (higher) decompressed limit.
func TestDecompressedReadLimit_LegitCompressible(t *testing.T) {
	const compressedReadLimit = 8 * 1024    // mimics MessageSizeLimit
	const decompressedReadLimit = 80 * 1024 // mimics MessageSizeLimit * multiplier
	const payloadSize = 64 * 1024           // > compressed limit, < decompressed limit

	payload := bytes.Repeat([]byte("centrifuge "), payloadSize/11+1)[:payloadSize]
	src := writeCompressedFrame(t, payload, 9)
	if src.Len() >= compressedReadLimit {
		t.Fatalf("compressed size %d not under compressed read limit %d", src.Len(), compressedReadLimit)
	}

	rc, out := newCompressedReader(src, decompressedReadLimit)
	rc.SetReadLimit(compressedReadLimit) // compressed-bytes limit also active

	_, r, err := rc.NextReader()
	if err != nil {
		t.Fatalf("NextReader: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("legit compressible message rejected: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("payload mismatch: got %d bytes, want %d", len(data), len(payload))
	}
	if sentTooBig(out) {
		t.Fatalf("unexpected CloseMessageTooBig for a legit message")
	}
}

// TestDecompressedReadLimit_ReproducesPreFixVulnerability documents the
// vulnerability the fix addresses: with only the compressed-bytes limit
// (SetReadLimit, the protection that existed before this change) and no
// decompressed limit, a small compressed frame that satisfies the compressed
// limit still inflates without bound. This is exactly the pre-fix behavior
// (decompressedReadLimit == 0 is equivalent to the code before the fix).
func TestDecompressedReadLimit_ReproducesPreFixVulnerability(t *testing.T) {
	const compressedReadLimit = 64 * 1024    // the old message_size_limit default
	const decompressedSize = 8 * 1024 * 1024 // 8MB from a few KB on the wire

	src := writeCompressedFrame(t, bytes.Repeat([]byte{'A'}, decompressedSize), 9)
	compressedLen := src.Len() // capture before reading drains the buffer
	// The compressed frame is comfortably under the compressed read limit, so
	// the pre-fix defense does not trigger.
	if compressedLen >= compressedReadLimit {
		t.Fatalf("compressed frame %d bytes not under compressed limit %d", compressedLen, compressedReadLimit)
	}

	rc, out := newCompressedReader(src, 0) // 0 == no decompressed limit (pre-fix)
	rc.SetReadLimit(compressedReadLimit)   // only the old compressed-bytes defense

	_, r, err := rc.NextReader()
	if err != nil {
		t.Fatalf("NextReader: %v", err)
	}
	data, err := io.ReadAll(r)
	// Pre-fix: no error and the full 8MB is materialized despite the compressed
	// read limit, demonstrating the unbounded amplification (the bug).
	if err != nil {
		t.Fatalf("pre-fix path unexpectedly errored: %v", err)
	}
	if len(data) != decompressedSize {
		t.Fatalf("inflated to %d bytes, want full %d (unbounded amplification)", len(data), decompressedSize)
	}
	if sentTooBig(out) {
		t.Fatalf("pre-fix path should not send CloseMessageTooBig")
	}
	t.Logf("reproduced: %d compressed wire bytes (<= %d limit) inflated to %d bytes unbounded",
		compressedLen, compressedReadLimit, len(data))
}

// TestDecompressedReadLimit_Disabled proves that a zero limit means unlimited
// (the fork primitive's contract): the bomb fully inflates without error.
func TestDecompressedReadLimit_Disabled(t *testing.T) {
	const decompressedSize = 1 * 1024 * 1024
	payload := bytes.Repeat([]byte{'A'}, decompressedSize)
	src := writeCompressedFrame(t, payload, 9)

	rc, out := newCompressedReader(src, 0) // 0 == unlimited

	_, r, err := rc.NextReader()
	if err != nil {
		t.Fatalf("NextReader: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("unexpected error with limit disabled: %v", err)
	}
	if len(data) != decompressedSize {
		t.Fatalf("read %d bytes, want %d", len(data), decompressedSize)
	}
	if sentTooBig(out) {
		t.Fatalf("unexpected CloseMessageTooBig with limit disabled")
	}
}

// TestDecompressedReadLimit_IgnoredWithoutCompression proves the decompressed
// limit only applies to compressed frames: an uncompressed message larger than
// the decompressed limit (but within the compressed read limit) passes through,
// because the limit is gated on the RSV1/decompress path.
func TestDecompressedReadLimit_IgnoredWithoutCompression(t *testing.T) {
	const decompressedReadLimit = 1024
	const payloadSize = 8 * 1024

	var src bytes.Buffer
	wc := newTestConn(nil, &src, false) // no compression writer -> plain frame
	w, err := wc.NextWriter(BinaryMessage)
	if err != nil {
		t.Fatalf("NextWriter: %v", err)
	}
	payload := bytes.Repeat([]byte{'y'}, payloadSize)
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	rc, _ := newCompressedReader(&src, decompressedReadLimit)
	// readLimit (compressed bytes) generous enough to admit the plain frame.
	rc.SetReadLimit(payloadSize + 1024)

	_, r, err := rc.NextReader()
	if err != nil {
		t.Fatalf("NextReader: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("uncompressed message wrongly limited: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("payload mismatch: got %d bytes, want %d", len(data), len(payload))
	}
}

// TestDecompressedReadLimit_CompressedLimitStillApplies proves the two limits
// are independent: with a decompressed limit set, the compressed-bytes limit
// enforced in advanceFrame still trips for an oversized compressed frame.
func TestDecompressedReadLimit_CompressedLimitStillApplies(t *testing.T) {
	const compressedReadLimit = 1024
	// Incompressible (deterministic pseudo-random) payload so compressed size
	// stays large and exceeds the compressed read limit before decompression
	// even matters.
	payload := make([]byte, 8*1024)
	rng := rand.New(rand.NewSource(1))
	_, _ = rng.Read(payload)
	src := writeCompressedFrame(t, payload, 0)
	if src.Len() <= compressedReadLimit {
		t.Fatalf("payload compressed too well (%d bytes) to exceed compressed limit", src.Len())
	}

	rc, _ := newCompressedReader(src, 10*1024*1024) // generous decompressed limit
	rc.SetReadLimit(compressedReadLimit)

	_, r, err := rc.NextReader()
	if err != nil {
		// Limit may surface from NextReader itself (advanceFrame) ...
		if !errors.Is(err, ErrReadLimit) {
			t.Fatalf("NextReader error = %v, want ErrReadLimit", err)
		}
		return
	}
	// ... or while reading.
	if _, err := io.ReadAll(r); !errors.Is(err, ErrReadLimit) {
		t.Fatalf("io.ReadAll error = %v, want ErrReadLimit", err)
	}
}

// TestDecompressedReadLimit_StreamingReads proves the limit is enforced on the
// incremental NextReader path (used by the bidirectional handler's streaming
// command decoder), not only via io.ReadAll: reading small chunks still trips
// the limit and keeps total consumed bytes bounded.
func TestDecompressedReadLimit_StreamingReads(t *testing.T) {
	const limit = 4096
	payload := bytes.Repeat([]byte{'z'}, 1*1024*1024)
	src := writeCompressedFrame(t, payload, 9)

	rc, out := newCompressedReader(src, limit)
	_, r, err := rc.NextReader()
	if err != nil {
		t.Fatalf("NextReader: %v", err)
	}

	buf := make([]byte, 256)
	var total int
	for {
		n, err := r.Read(buf)
		total += n
		if err != nil {
			if !errors.Is(err, ErrReadLimit) {
				t.Fatalf("Read error = %v, want ErrReadLimit", err)
			}
			break
		}
		if total > limit+1 {
			t.Fatalf("consumed %d bytes without hitting limit %d", total, limit)
		}
	}
	if total > limit+1 {
		t.Fatalf("consumed %d bytes, want at most %d", total, limit+1)
	}
	if !sentTooBig(out) {
		t.Fatalf("expected CloseMessageTooBig control frame to be sent")
	}
}
