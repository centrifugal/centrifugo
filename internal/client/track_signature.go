package client

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"strconv"
	"strings"
	"sync"
)

const gracePeriodSeconds = 5

func parseSignatureTimestamps(sig string) (iat int64, expiry int64) {
	first := strings.IndexByte(sig, ':')
	if first < 0 {
		return 0, 0
	}
	rest := sig[first+1:]
	second := strings.IndexByte(rest, ':')
	if second < 0 {
		return 0, 0
	}
	iatVal, err := strconv.ParseInt(sig[:first], 10, 64)
	if err != nil {
		return 0, 0
	}
	expiryVal, err := strconv.ParseInt(rest[:second], 10, 64)
	if err != nil {
		return 0, 0
	}
	return iatVal, expiryVal
}

// trackSignatureVerifier reuses HMAC hashers and buffers for signature verification.
// Thread-safe via sync.Pool.
type trackSignatureVerifier struct {
	macPool sync.Pool
	bufPool sync.Pool
}

type verifyBufs struct {
	payload     []byte
	hexBuf      []byte
	expectedHex []byte
}

func newTrackSignatureVerifier(secret string) *trackSignatureVerifier {
	return &trackSignatureVerifier{
		macPool: sync.Pool{
			New: func() any {
				return hmac.New(sha256.New, []byte(secret))
			},
		},
		bufPool: sync.Pool{
			New: func() any {
				return &verifyBufs{
					payload:     make([]byte, 0, 256),
					hexBuf:      make([]byte, hex.EncodedLen(sha256.Size)),
					expectedHex: make([]byte, hex.EncodedLen(sha256.Size)),
				}
			},
		},
	}
}

func verifyTrackSignature(secret string, channel string, sig string, keys []string, userID string) bool {
	return newTrackSignatureVerifier(secret).verify(channel, sig, keys, userID)
}

func (v *trackSignatureVerifier) verify(channel string, sig string, keys []string, userID string) bool {
	first := strings.IndexByte(sig, ':')
	if first < 0 {
		return false
	}
	rest := sig[first+1:]
	second := strings.IndexByte(rest, ':')
	if second < 0 {
		return false
	}
	iatStr := sig[:first]
	expiryStr := rest[:second]
	hmacHex := rest[second+1:]

	keysHash := sha256.Sum256([]byte(strings.Join(keys, "\x00")))

	mac := v.macPool.Get().(hash.Hash)
	bufs := v.bufPool.Get().(*verifyBufs)

	mac.Reset()
	bufs.payload = bufs.payload[:0]
	bufs.payload = append(bufs.payload, iatStr...)
	bufs.payload = append(bufs.payload, ':')
	bufs.payload = append(bufs.payload, expiryStr...)
	bufs.payload = append(bufs.payload, ':')
	bufs.payload = append(bufs.payload, userID...)
	bufs.payload = append(bufs.payload, ':')
	bufs.payload = append(bufs.payload, channel...)
	bufs.payload = append(bufs.payload, ':')
	hex.Encode(bufs.hexBuf, keysHash[:])
	bufs.payload = append(bufs.payload, bufs.hexBuf...)
	mac.Write(bufs.payload)
	hex.Encode(bufs.expectedHex, mac.Sum(nil))

	result := hmac.Equal([]byte(hmacHex), bufs.expectedHex)

	v.macPool.Put(mac)
	v.bufPool.Put(bufs)

	return result
}
