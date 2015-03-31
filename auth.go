package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
)

func generateClientToken(secretKey, projectKey, userId, timestamp, info string) string {
	token := hmac.New(sha256.New, []byte(secretKey))
	token.Write([]byte(projectKey))
	token.Write([]byte(userId))
	token.Write([]byte(timestamp))
	token.Write([]byte(info))
	return fmt.Sprintf("%02x", token.Sum(nil))
}

func checkClientToken(secretKey, projectKey, userId, timestamp, info, providedToken string) bool {
	token := generateClientToken(secretKey, projectKey, userId, timestamp, info)
	return token == providedToken
}

func generateApiSign(secretKey, projectKey, encodedData string) string {
	sign := hmac.New(sha256.New, []byte(secretKey))
	sign.Write([]byte(projectKey))
	sign.Write([]byte(encodedData))
	return fmt.Sprintf("%02x", sign.Sum(nil))
}

func checkApiSign(secretKey, projectKey, encodedData, providedSign string) bool {
	sign := generateApiSign(secretKey, projectKey, encodedData)
	return sign == providedSign
}

func generateChannelSign(secretKey, clientId, channel, channelData string) string {
	sign := hmac.New(sha256.New, []byte(secretKey))
	sign.Write([]byte(clientId))
	sign.Write([]byte(channel))
	sign.Write([]byte(channelData))
	return fmt.Sprintf("%02x", sign.Sum(nil))
}

func checkChannelSign(secretKey, clientId, channel, channelData, providedSign string) bool {
	sign := generateChannelSign(secretKey, clientId, channel, channelData)
	return sign == providedSign
}
