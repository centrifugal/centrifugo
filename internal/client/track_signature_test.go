package client

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-key"

func makeTestSignature(secret, channel string, keys []string, userID string, iat, expiry int64) string {
	sortedKeys := make([]string, len(keys))
	copy(sortedKeys, keys)
	sort.Strings(sortedKeys)
	keysHash := sha256.Sum256([]byte(strings.Join(sortedKeys, "\x00")))
	payload := fmt.Sprintf("%d:%d:%s:%s:%x", iat, expiry, userID, channel, keysHash)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return fmt.Sprintf("%d:%d:%x", iat, expiry, mac.Sum(nil))
}

func TestTrackSignature_ValidHMAC(t *testing.T) {
	now := time.Now().Unix()
	expiry := now + 3600
	sig := makeTestSignature(testSecret, "test:channel", []string{"key1", "key2"}, "user1", now, expiry)
	require.True(t, verifyTrackSignature(testSecret, "test:channel", sig, []string{"key1", "key2"}, "user1"))
}

func TestTrackSignature_InvalidHMAC(t *testing.T) {
	now := time.Now().Unix()
	expiry := now + 3600
	sig := makeTestSignature(testSecret, "test:channel", []string{"key1", "key2"}, "user1", now, expiry)
	// Tamper with the signature.
	sig = sig[:len(sig)-2] + "ff"
	require.False(t, verifyTrackSignature(testSecret, "test:channel", sig, []string{"key1", "key2"}, "user1"))
}

func TestTrackSignature_ExpiredSignature(t *testing.T) {
	now := time.Now().Unix()
	iat := now - 7200
	expiry := now - 3600 // Expired 1 hour ago.
	sig := makeTestSignature(testSecret, "test:channel", []string{"key1"}, "user1", iat, expiry)

	// Signature is valid HMAC-wise but the caller (OnKeyedTrack) would check expiry.
	// verifyTrackSignature itself only checks HMAC, not expiry.
	require.True(t, verifyTrackSignature(testSecret, "test:channel", sig, []string{"key1"}, "user1"))

	// But parseSignatureTimestamps should return the expired value.
	_, parsedExpiry := parseSignatureTimestamps(sig)
	require.True(t, parsedExpiry < now)
}

func TestTrackSignature_DifferentKeys(t *testing.T) {
	now := time.Now().Unix()
	expiry := now + 3600
	sig := makeTestSignature(testSecret, "test:channel", []string{"keyA", "keyB"}, "user1", now, expiry)
	// Client sends different keys — should fail.
	require.False(t, verifyTrackSignature(testSecret, "test:channel", sig, []string{"keyA", "keyC"}, "user1"))
}

func TestTrackSignature_DifferentUser(t *testing.T) {
	now := time.Now().Unix()
	expiry := now + 3600
	sig := makeTestSignature(testSecret, "test:channel", []string{"key1"}, "user1", now, expiry)
	// Different user — should fail.
	require.False(t, verifyTrackSignature(testSecret, "test:channel", sig, []string{"key1"}, "user2"))
}

func TestTrackSignature_AnonymousUser(t *testing.T) {
	now := time.Now().Unix()
	expiry := now + 3600
	sig := makeTestSignature(testSecret, "test:channel", []string{"key1"}, "", now, expiry)
	require.True(t, verifyTrackSignature(testSecret, "test:channel", sig, []string{"key1"}, ""))
}

func TestTrackSignature_KeyOrderIndependent(t *testing.T) {
	now := time.Now().Unix()
	expiry := now + 3600
	// Sign with keys [B, A].
	sig := makeTestSignature(testSecret, "test:channel", []string{"keyB", "keyA"}, "user1", now, expiry)
	// Verify with keys [A, B] — should succeed because both sides sort.
	require.True(t, verifyTrackSignature(testSecret, "test:channel", sig, []string{"keyA", "keyB"}, "user1"))
}

func TestTrackSignature_EmptySignature(t *testing.T) {
	require.False(t, verifyTrackSignature(testSecret, "test:channel", "", []string{"key1"}, "user1"))
}

func TestTrackSignature_MalformedSignature(t *testing.T) {
	require.False(t, verifyTrackSignature(testSecret, "test:channel", "not-a-valid-sig", []string{"key1"}, "user1"))
	require.False(t, verifyTrackSignature(testSecret, "test:channel", "123", []string{"key1"}, "user1"))
}

func TestTrackSignature_WrongSecret(t *testing.T) {
	now := time.Now().Unix()
	expiry := now + 3600
	sig := makeTestSignature(testSecret, "test:channel", []string{"key1"}, "user1", now, expiry)
	require.False(t, verifyTrackSignature("wrong-secret", "test:channel", sig, []string{"key1"}, "user1"))
}

func TestTrackSignature_WrongChannel(t *testing.T) {
	now := time.Now().Unix()
	expiry := now + 3600
	sig := makeTestSignature(testSecret, "test:channel", []string{"key1"}, "user1", now, expiry)
	require.False(t, verifyTrackSignature(testSecret, "other:channel", sig, []string{"key1"}, "user1"))
}

func TestParseSignatureTimestamps(t *testing.T) {
	iat, expiry := parseSignatureTimestamps("1000:2000:abcdef")
	require.Equal(t, int64(1000), iat)
	require.Equal(t, int64(2000), expiry)
}

func TestParseSignatureTimestamps_Invalid(t *testing.T) {
	tests := []string{
		"",
		"abc",
		"1000",
		"1000:",
		"1000:abc:def",
		"abc:2000:def",
	}
	for _, sig := range tests {
		iat, expiry := parseSignatureTimestamps(sig)
		require.Equal(t, int64(0), iat, "sig=%q", sig)
		require.Equal(t, int64(0), expiry, "sig=%q", sig)
	}
}

func TestTrackSignature_SingleKey(t *testing.T) {
	now := time.Now().Unix()
	expiry := now + 3600
	sig := makeTestSignature(testSecret, "ch", []string{"only-key"}, "u", now, expiry)
	require.True(t, verifyTrackSignature(testSecret, "ch", sig, []string{"only-key"}, "u"))
}

func TestTrackSignature_ManyKeys(t *testing.T) {
	now := time.Now().Unix()
	expiry := now + 3600
	keys := make([]string, 100)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%03d", i)
	}
	sig := makeTestSignature(testSecret, "ch", keys, "user1", now, expiry)
	require.True(t, verifyTrackSignature(testSecret, "ch", sig, keys, "user1"))
}

func TestParseSignatureTimestamps_RealSignature(t *testing.T) {
	now := time.Now().Unix()
	expiry := now + 3600
	sig := makeTestSignature(testSecret, "ch", []string{"k"}, "u", now, expiry)
	parsedIat, parsedExpiry := parseSignatureTimestamps(sig)
	require.Equal(t, now, parsedIat)
	require.Equal(t, expiry, parsedExpiry)
}

func TestTrackSignature_GracePeriod(t *testing.T) {
	now := time.Now().Unix()
	// Expired 3 seconds ago — within 5-second grace period.
	iat := now - 3600
	expiry := now - 3
	sig := makeTestSignature(testSecret, "test:channel", []string{"key1"}, "user1", iat, expiry)

	// HMAC is valid (verifyTrackSignature doesn't check expiry).
	require.True(t, verifyTrackSignature(testSecret, "test:channel", sig, []string{"key1"}, "user1"))

	// The caller would check: expiry + gracePeriodSeconds >= now
	_, parsedExpiry := parseSignatureTimestamps(sig)
	withinGracePeriod := parsedExpiry+gracePeriodSeconds >= now
	require.True(t, withinGracePeriod)

	// Outside grace period.
	iat2 := now - 7200
	expiry2 := now - 60 // Expired 60 seconds ago, past grace period.
	sig2 := makeTestSignature(testSecret, "test:channel", []string{"key1"}, "user1", iat2, expiry2)
	_, parsedExpiry2 := parseSignatureTimestamps(sig2)
	withinGracePeriod2 := parsedExpiry2+gracePeriodSeconds >= now
	require.False(t, withinGracePeriod2)
}

func TestVerifyTrackSignature_HexEncoding(t *testing.T) {
	// Verify that the expected hex encoding matches between make and verify.
	now := time.Now().Unix()
	expiry := now + 3600
	keys := []string{"key1"}
	sortedKeys := make([]string, len(keys))
	copy(sortedKeys, keys)
	sort.Strings(sortedKeys)
	keysHash := sha256.Sum256([]byte(strings.Join(sortedKeys, "\x00")))
	payload := fmt.Sprintf("%d:%d:%s:%s:%x", now, expiry, "u", "ch", keysHash)
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(payload))
	expectedHex := hex.EncodeToString(mac.Sum(nil))

	sig := fmt.Sprintf("%d:%d:%s", now, expiry, expectedHex)
	require.True(t, verifyTrackSignature(testSecret, "ch", sig, keys, "u"))
}
