// Package auth provides functions to generate and check Centrifugo tokens and signs.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"github.com/gorilla/securecookie"
)

// HMACLength used to validate length of token string we received.
// Centrifugo uses sha256 as digest algorithm for HMAC tokens and signs
// so all correct tokens must be of fixed length set by this const.
const (
	HMACLength = 64
)

// GenerateClientToken generates client token based on project secret key and provided
// connection parameters such as user ID, timestamp and info JSON string.
func GenerateClientToken(secret, user, timestamp, info string) string {
	token := hmac.New(sha256.New, []byte(secret))
	token.Write([]byte(user))
	token.Write([]byte(timestamp))
	token.Write([]byte(info))
	return hex.EncodeToString(token.Sum(nil))
}

// CheckClientToken validates correctness of provided (by client connection) token
// comparing it with generated one
func CheckClientToken(secret, user, timestamp, info, providedToken string) bool {
	if len(providedToken) != HMACLength {
		return false
	}
	token := GenerateClientToken(secret, user, timestamp, info)
	return hmac.Equal([]byte(token), []byte(providedToken))
}

// GenerateApiSign generates sign which is used to sign HTTP API requests
func GenerateApiSign(secret string, data []byte) string {
	sign := hmac.New(sha256.New, []byte(secret))
	sign.Write(data)
	return hex.EncodeToString(sign.Sum(nil))
}

// CheckApiSign validates correctness of provided (in HTTP API request) sign
// comparing it with generated one
func CheckApiSign(secret string, data []byte, providedSign string) bool {
	if len(providedSign) != HMACLength {
		return false
	}
	sign := GenerateApiSign(secret, data)
	return hmac.Equal([]byte(sign), []byte(providedSign))
}

// GenerateChannelSign generates sign which is used to prove permission of
// client to subscribe on private channel
func GenerateChannelSign(secret, client, channel, channelData string) string {
	sign := hmac.New(sha256.New, []byte(secret))
	sign.Write([]byte(client))
	sign.Write([]byte(channel))
	sign.Write([]byte(channelData))
	return hex.EncodeToString(sign.Sum(nil))
}

// CheckChannelSign validates a correctness of provided (in subscribe client command)
// sign comparing it with generated one
func CheckChannelSign(secret, client, channel, channelData, providedSign string) bool {
	if len(providedSign) != HMACLength {
		return false
	}
	sign := GenerateChannelSign(secret, client, channel, channelData)
	return hmac.Equal([]byte(sign), []byte(providedSign))
}

const (
	// AdminTokenKey is a key for admin authorization token.
	AdminTokenKey = "token"
	// AdminTokenValue is a value for secure admin authorization token.
	AdminTokenValue = "authorized"
)

// GenerateAdminToken generates admin authentication token.
func GenerateAdminToken(secret string) (string, error) {
	s := securecookie.New([]byte(secret), nil)
	return s.Encode(AdminTokenKey, AdminTokenValue)
}

// CheckAdminToken checks admin connection token which Centrifugo returns after admin login.
func CheckAdminToken(secret string, token string) bool {
	s := securecookie.New([]byte(secret), nil)
	var val string
	err := s.Decode(AdminTokenKey, token, &val)
	if err != nil {
		return false
	}
	if val != AdminTokenValue {
		return false
	}
	return true
}
