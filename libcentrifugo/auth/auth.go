package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
)

// GenerateClientToken generates client token based on project secret key and provided
// connection parameters such as user ID, timestamp and info JSON string.
func GenerateClientToken(secretKey, projectKey, user, timestamp, info string) string {
	token := hmac.New(sha256.New, []byte(secretKey))
	token.Write([]byte(projectKey))
	token.Write([]byte(user))
	token.Write([]byte(timestamp))
	if info == "" {
		info = "{}"
	}
	token.Write([]byte(info))
	return fmt.Sprintf("%02x", token.Sum(nil))
}

// CheckClientToken validates correctness of provided (by client connection) token
// comparing it with generated one
func CheckClientToken(secretKey, projectKey, user, timestamp, info, providedToken string) bool {
	token := GenerateClientToken(secretKey, projectKey, user, timestamp, info)
	return token == providedToken
}

// GenerateApiSign generates sign which is used to sign HTTP API requests
func GenerateApiSign(secretKey, projectKey, encodedData string) string {
	sign := hmac.New(sha256.New, []byte(secretKey))
	sign.Write([]byte(projectKey))
	sign.Write([]byte(encodedData))
	return fmt.Sprintf("%02x", sign.Sum(nil))
}

// CheckApiSign validates correctness of provided (in HTTP API request) sign
// comparing it with generated one
func CheckApiSign(secretKey, projectKey, encodedData, providedSign string) bool {
	sign := GenerateApiSign(secretKey, projectKey, encodedData)
	return sign == providedSign
}

// GenerateChannelSign generates sign which is used to prove permission of
// client to subscribe on private channel
func GenerateChannelSign(secretKey, client, channel, channelData string) string {
	sign := hmac.New(sha256.New, []byte(secretKey))
	sign.Write([]byte(client))
	sign.Write([]byte(channel))
	sign.Write([]byte(channelData))
	return fmt.Sprintf("%02x", sign.Sum(nil))
}

// CheckChannelSign validates a correctness of provided (in subscribe client command)
// sign comparing it with generated one
func CheckChannelSign(secretKey, client, channel, channelData, providedSign string) bool {
	sign := GenerateChannelSign(secretKey, client, channel, channelData)
	return sign == providedSign
}
