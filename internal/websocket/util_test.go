// Copyright 2014 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package websocket

import (
	"crypto/sha1"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

var equalASCIIFoldTests = []struct {
	t, s string
	eq   bool
}{
	{"WebSocket", "websocket", true},
	{"websocket", "WebSocket", true},
	{"Öyster", "öyster", false},
	{"WebSocket", "WetSocket", false},
}

func TestEqualASCIIFold(t *testing.T) {
	for _, tt := range equalASCIIFoldTests {
		eq := equalASCIIFold(tt.s, tt.t)
		if eq != tt.eq {
			t.Errorf("equalASCIIFold(%q, %q) = %v, want %v", tt.s, tt.t, eq, tt.eq)
		}
	}
}

var tokenListContainsValueTests = []struct {
	value string
	ok    bool
}{
	{"WebSocket", true},
	{"WEBSOCKET", true},
	{"websocket", true},
	{"websockets", false},
	{"x websocket", false},
	{"websocket x", false},
	{"other,websocket,more", true},
	{"other, websocket, more", true},
}

func TestTokenListContainsValue(t *testing.T) {
	for _, tt := range tokenListContainsValueTests {
		h := http.Header{"Upgrade": {tt.value}}
		ok := tokenListContainsValue(h, "Upgrade", "websocket")
		if ok != tt.ok {
			t.Errorf("tokenListContainsValue(h, n, %q) = %v, want %v", tt.value, ok, tt.ok)
		}
	}
}

var isValidChallengeKeyTests = []struct {
	key string
	ok  bool
}{
	{"dGhlIHNhbXBsZSBub25jZQ==", true},
	{"", false},
	{"InvalidKey", false},
	{"WHQ4eXhscUtKYjBvOGN3WEdtOEQ=", false},
}

func TestIsValidChallengeKey(t *testing.T) {
	for _, tt := range isValidChallengeKeyTests {
		ok := isValidChallengeKey(tt.key)
		if ok != tt.ok {
			t.Errorf("isValidChallengeKey returns %v, want %v", ok, tt.ok)
		}
	}
}

var parseExtensionTests = []struct {
	value      string
	extensions []map[string]string
}{
	{`foo`, []map[string]string{{"": "foo"}}},
	{`foo, bar; baz=2`, []map[string]string{
		{"": "foo"},
		{"": "bar", "baz": "2"}}},
	{`foo; bar="b,a;z"`, []map[string]string{
		{"": "foo", "bar": "b,a;z"}}},
	{`foo , bar; baz = 2`, []map[string]string{
		{"": "foo"},
		{"": "bar", "baz": "2"}}},
	{`foo, bar; baz=2 junk`, []map[string]string{
		{"": "foo"}}},
	{`foo junk, bar; baz=2 junk`, nil},
	{`mux; max-channels=4; flow-control, deflate-stream`, []map[string]string{
		{"": "mux", "max-channels": "4", "flow-control": ""},
		{"": "deflate-stream"}}},
	{`permessage-foo; x="10"`, []map[string]string{
		{"": "permessage-foo", "x": "10"}}},
	{`permessage-foo; use_y, permessage-foo`, []map[string]string{
		{"": "permessage-foo", "use_y": ""},
		{"": "permessage-foo"}}},
	{`permessage-deflate; client_max_window_bits; server_max_window_bits=10 , permessage-deflate; client_max_window_bits`, []map[string]string{
		{"": "permessage-deflate", "client_max_window_bits": "", "server_max_window_bits": "10"},
		{"": "permessage-deflate", "client_max_window_bits": ""}}},
	{"permessage-deflate; server_no_context_takeover; client_max_window_bits=15", []map[string]string{
		{"": "permessage-deflate", "server_no_context_takeover": "", "client_max_window_bits": "15"},
	}},
}

func TestParseExtensions(t *testing.T) {
	for _, tt := range parseExtensionTests {
		h := http.Header{http.CanonicalHeaderKey("Sec-WebSocket-Extensions"): {tt.value}}
		extensions := parseExtensions(h)
		if !reflect.DeepEqual(extensions, tt.extensions) {
			t.Errorf("parseExtensions(%q)\n    = %v,\nwant %v", tt.value, extensions, tt.extensions)
		}
	}
}

func TestEncodeAcceptKey(t *testing.T) {
	challengeKey := "dGhlIHNhbXBsZSBub25jZQ=="
	expectedKey := computeAcceptKey(challengeKey)

	// Initial encoding.
	result := encodeAcceptKey(challengeKey, []byte{})
	if !strings.EqualFold(expectedKey, string(result)) {
		t.Errorf("Expected %s, got %s", expectedKey, string(result))
	}

	// Test reuse of pooled buffer to see if it correctly resets and does not grow unexpectedly.
	result2 := encodeAcceptKey(challengeKey, []byte{})
	if len(result2) != len(result) {
		t.Errorf("Buffer reused improperly, expected length %d, got length %d", len(result), len(result2))
	}

	// Test that we really append to the buffer.
	result3 := encodeAcceptKey(challengeKey, []byte{0})
	if result3[0] != 0 {
		t.Errorf("appended improperly, expected 0, got %d", result3[0])
	}
	if string(result3[1:]) != expectedKey {
		t.Errorf("Expected %s, got %s", expectedKey, string(result3[1:]))
	}

	// Check if buffer returns the same size after multiple uses
	for i := 0; i < 10; i++ {
		_ = encodeAcceptKey(challengeKey, []byte{})
	}
	bufPtr := acceptKeyBufferPool.Get().(*[]byte)
	if cap(*bufPtr) != sha1.Size {
		t.Errorf("Expected buffer capacity to be %d, got %d", sha1.Size, cap(*bufPtr))
	}
	acceptKeyBufferPool.Put(bufPtr)
}
