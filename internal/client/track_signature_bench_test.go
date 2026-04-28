package client

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cristalhq/jwt/v5"
	sjson "github.com/segmentio/encoding/json"
)

func generateKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("item_%05d", i)
	}
	return keys
}

// HMAC signature: create.
func createTrackSignature(secret, channel string, keys []string, userID string, iat, expiry int64) string {
	keysHash := sha256.Sum256([]byte(strings.Join(keys, "\x00")))
	payload := fmt.Sprintf("%d:%d:%s:%s:%x", iat, expiry, userID, channel, keysHash)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return fmt.Sprintf("%d:%d:%x", iat, expiry, mac.Sum(nil))
}

// JWT: claims struct that includes keys.
type trackClaims struct {
	jwt.RegisteredClaims
	Channel string   `json:"channel"`
	Keys    []string `json:"keys"`
}

func BenchmarkTrackSignature_HMAC_Create_10kKeys(b *testing.B) {
	keys := generateKeys(10_000)
	secret := "bench-secret-key"
	channel := "post_votes:feed1"
	userID := "user123"
	now := time.Now().Unix()
	expiry := now + 3600

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		createTrackSignature(secret, channel, keys, userID, now, expiry)
	}
}

func BenchmarkTrackSignature_HMAC_Create_Reuse_10kKeys(b *testing.B) {
	keys := generateKeys(10_000)
	secret := "bench-secret-key"
	channel := "post_votes:feed1"
	userID := "user123"
	now := time.Now().Unix()
	expiry := now + 3600

	// Pre-compute keys hash.
	keysHash := sha256.Sum256([]byte(strings.Join(keys, "\x00")))

	// Reuse HMAC hasher and buffers.
	mac := hmac.New(sha256.New, []byte(secret))
	payload := make([]byte, 0, 256)
	result := make([]byte, 0, 128)
	hexBuf := make([]byte, hex.EncodedLen(sha256.Size))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mac.Reset()
		payload = payload[:0]
		payload = strconv.AppendInt(payload, now, 10)
		payload = append(payload, ':')
		payload = strconv.AppendInt(payload, expiry, 10)
		payload = append(payload, ':')
		payload = append(payload, userID...)
		payload = append(payload, ':')
		payload = append(payload, channel...)
		payload = append(payload, ':')
		hex.Encode(hexBuf, keysHash[:])
		payload = append(payload, hexBuf...)
		mac.Write(payload)

		result = result[:0]
		result = strconv.AppendInt(result, now, 10)
		result = append(result, ':')
		result = strconv.AppendInt(result, expiry, 10)
		result = append(result, ':')
		hex.Encode(hexBuf, mac.Sum(nil))
		result = append(result, hexBuf...)
		_ = string(result)
	}
}

func BenchmarkTrackSignature_HMAC_Verify_10kKeys(b *testing.B) {
	keys := generateKeys(10_000)
	secret := "bench-secret-key"
	channel := "post_votes:feed1"
	userID := "user123"
	now := time.Now().Unix()
	expiry := now + 3600
	sig := createTrackSignature(secret, channel, keys, userID, now, expiry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !verifyTrackSignature(secret, channel, sig, keys, userID) {
			b.Fatal("verification failed")
		}
	}
}

func BenchmarkTrackSignature_HMAC_Verify_Reuse_10kKeys(b *testing.B) {
	keys := generateKeys(10_000)
	secret := "bench-secret-key"
	channel := "post_votes:feed1"
	userID := "user123"
	now := time.Now().Unix()
	expiry := now + 3600
	sig := createTrackSignature(secret, channel, keys, userID, now, expiry)

	// Pre-compute keys hash.
	keysHash := sha256.Sum256([]byte(strings.Join(keys, "\x00")))

	// Reuse HMAC hasher and buffers.
	mac := hmac.New(sha256.New, []byte(secret))
	payload := make([]byte, 0, 256)
	hexBuf := make([]byte, hex.EncodedLen(sha256.Size))
	expectedHex := make([]byte, hex.EncodedLen(sha256.Size))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Parse iat:expiry:hmac from signature.
		first := strings.IndexByte(sig, ':')
		rest := sig[first+1:]
		second := strings.IndexByte(rest, ':')
		iatStr := sig[:first]
		expiryStr := rest[:second]
		hmacHex := rest[second+1:]

		mac.Reset()
		payload = payload[:0]
		payload = append(payload, iatStr...)
		payload = append(payload, ':')
		payload = append(payload, expiryStr...)
		payload = append(payload, ':')
		payload = append(payload, userID...)
		payload = append(payload, ':')
		payload = append(payload, channel...)
		payload = append(payload, ':')
		hex.Encode(hexBuf, keysHash[:])
		payload = append(payload, hexBuf...)
		mac.Write(payload)
		hex.Encode(expectedHex, mac.Sum(nil))

		if !hmac.Equal([]byte(hmacHex), expectedHex) {
			b.Fatal("verification failed")
		}
	}
}

func BenchmarkTrackSignature_JWT_Create_10kKeys(b *testing.B) {
	keys := generateKeys(10_000)
	secret := "bench-secret-key"
	channel := "post_votes:feed1"
	userID := "user123"
	now := time.Now()
	expiry := now.Add(time.Hour)

	signer, err := jwt.NewSignerHS(jwt.HS256, []byte(secret))
	if err != nil {
		b.Fatal(err)
	}
	builder := jwt.NewBuilder(signer)

	claims := &trackClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiry),
		},
		Channel: channel,
		Keys:    keys,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := builder.Build(claims)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTrackSignature_JWT_Create_Segmentio_10kKeys(b *testing.B) {
	keys := generateKeys(10_000)
	secret := "bench-secret-key"
	channel := "post_votes:feed1"
	userID := "user123"
	now := time.Now()
	expiry := now.Add(time.Hour)

	signer, err := jwt.NewSignerHS(jwt.HS256, []byte(secret))
	if err != nil {
		b.Fatal(err)
	}
	builder := jwt.NewBuilder(signer)

	claims := &trackClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiry),
		},
		Channel: channel,
		Keys:    keys,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rawClaims, err := sjson.Marshal(claims)
		if err != nil {
			b.Fatal(err)
		}
		_, err = builder.Build(rawClaims)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTrackSignature_JWT_Verify_10kKeys(b *testing.B) {
	keys := generateKeys(10_000)
	secret := "bench-secret-key"
	channel := "post_votes:feed1"
	userID := "user123"
	now := time.Now()
	expiry := now.Add(time.Hour)

	signer, err := jwt.NewSignerHS(jwt.HS256, []byte(secret))
	if err != nil {
		b.Fatal(err)
	}
	builder := jwt.NewBuilder(signer)

	claims := &trackClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiry),
		},
		Channel: channel,
		Keys:    keys,
	}

	token, err := builder.Build(claims)
	if err != nil {
		b.Fatal(err)
	}
	tokenBytes := token.Bytes()

	verifier, err := jwt.NewVerifierHS(jwt.HS256, []byte(secret))
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parsedToken, err := jwt.Parse(tokenBytes, verifier)
		if err != nil {
			b.Fatal(err)
		}
		var parsed trackClaims
		if err := parsedToken.DecodeClaims(&parsed); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTrackSignature_JWT_Verify_Segmentio_10kKeys(b *testing.B) {
	keys := generateKeys(10_000)
	secret := "bench-secret-key"
	channel := "post_votes:feed1"
	userID := "user123"
	now := time.Now()
	expiry := now.Add(time.Hour)

	signer, err := jwt.NewSignerHS(jwt.HS256, []byte(secret))
	if err != nil {
		b.Fatal(err)
	}
	builder := jwt.NewBuilder(signer)

	claims := &trackClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiry),
		},
		Channel: channel,
		Keys:    keys,
	}

	token, err := builder.Build(claims)
	if err != nil {
		b.Fatal(err)
	}
	tokenBytes := token.Bytes()

	verifier, err := jwt.NewVerifierHS(jwt.HS256, []byte(secret))
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parsedToken, err := jwt.Parse(tokenBytes, verifier)
		if err != nil {
			b.Fatal(err)
		}
		var parsed trackClaims
		if _, err := sjson.Parse(parsedToken.Claims(), &parsed, sjson.ZeroCopy); err != nil {
			b.Fatal(err)
		}
	}
}
