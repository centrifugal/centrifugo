// Package auth provides functions to generate and check Centrifugo auth signs.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// HMACLength used to validate length of token string we received.
// Centrifugo uses sha256 as digest algorithm for HMAC tokens and signs
// so all correct tokens must be of fixed length set by this const.
const (
	HMACLength = 64
)

// GenerateClientSign generates client connection sign based on secret key
// and provided connection parameters such as user ID, exp, info and opts.
func GenerateClientSign(secret, user, exp, info, opts string) string {
	token := hmac.New(sha256.New, []byte(secret))
	token.Write([]byte(user))
	token.Write([]byte(exp))
	token.Write([]byte(info))
	token.Write([]byte(opts))
	return hex.EncodeToString(token.Sum(nil))
}

// CheckClientSign validates correctness of sign provided by client connection
// comparing it with generated correct one.
func CheckClientSign(secret, user, exp, info, opts, providedToken string) bool {
	if len(providedToken) != HMACLength {
		return false
	}
	token := GenerateClientSign(secret, user, exp, info, opts)
	return hmac.Equal([]byte(token), []byte(providedToken))
}

// GenerateChannelSign generates sign which is used to prove permission of
// client to subscribe on private channel.
func GenerateChannelSign(secret, client, channel, channelData string) string {
	sign := hmac.New(sha256.New, []byte(secret))
	sign.Write([]byte(client))
	sign.Write([]byte(channel))
	sign.Write([]byte(channelData))
	return hex.EncodeToString(sign.Sum(nil))
}

// CheckChannelSign validates a correctness of provided (in subscribe client command)
// sign comparing it with generated one.
func CheckChannelSign(secret, client, channel, channelData, providedSign string) bool {
	if len(providedSign) != HMACLength {
		return false
	}
	sign := GenerateChannelSign(secret, client, channel, channelData)
	return hmac.Equal([]byte(sign), []byte(providedSign))
}
