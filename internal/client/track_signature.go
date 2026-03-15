package client

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const gracePeriodSeconds = 30

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

func verifyTrackSignature(secret string, channel string, sig string, keys []string, userID string) bool {
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

	sortedKeys := make([]string, len(keys))
	copy(sortedKeys, keys)
	sort.Strings(sortedKeys)
	keysHash := sha256.Sum256([]byte(strings.Join(sortedKeys, "\x00")))

	payload := fmt.Sprintf("%s:%s:%x:%s:%s", channel, userID, keysHash, iatStr, expiryStr)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expectedHex := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(hmacHex), []byte(expectedHex))
}
