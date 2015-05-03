package libcentrifugo

import (
	"testing"
)

const (
	secretKey   = "secret"
	projectKey  = "project"
	user        = "user"
	timestamp   = "1430669930"
	info        = "{}"
	encodedData = "{}"
	client      = "client"
	channel     = "channel"
	channelData = "{}"
)

func TestGenerateClientToken(t *testing.T) {
	tokenWithInfo := generateClientToken(secretKey, projectKey, user, timestamp, "{}")
	if len(tokenWithInfo) != 64 {
		t.Error("sha256 token length must be 64")
	}
	tokenWithoutInfo := generateClientToken(secretKey, projectKey, user, timestamp, "")
	if len(tokenWithoutInfo) != 64 {
		t.Error("sha256 token length must be 64")
	}
	if tokenWithInfo != tokenWithoutInfo {
		t.Error("token with empty info must be equal to token where info is empty object")
	}
}

func TestCheckClientToken(t *testing.T) {
	providedToken := "token"
	result := checkClientToken(secretKey, projectKey, user, timestamp, info, providedToken)
	if result {
		t.Error("provided token is wrong, but check passed")
	}
	correctToken := generateClientToken(secretKey, projectKey, user, timestamp, info)
	result = checkClientToken(secretKey, projectKey, user, timestamp, info, correctToken)
	if !result {
		t.Error("correct client token must pass check")
	}
}

func TestGenerateApiSign(t *testing.T) {
	sign := generateApiSign(secretKey, projectKey, encodedData)
	if len(sign) != 64 {
		t.Error("sha256 sign length must be 64")
	}
}

func TestCheckApiSign(t *testing.T) {
	providedSign := "sign"
	result := checkApiSign(secretKey, projectKey, encodedData, providedSign)
	if result {
		t.Error("provided sign is wrong, but check passed")
	}
	correctSign := generateApiSign(secretKey, projectKey, encodedData)
	result = checkApiSign(secretKey, projectKey, encodedData, correctSign)
	if !result {
		t.Error("correct sign must pass check")
	}
}

func TestGenerateChannelSign(t *testing.T) {
	sign := generateChannelSign(secretKey, clientId, channel, channelData)
	if len(sign) != 64 {
		t.Error("sha256 sign length must be 64")
	}
}

func TestCheckChannelSign(t *testing.T) {
	providedSign := "sign"
	result := checkChannelSign(secretKey, client, channel, channelData, providedSign)
	if result {
		t.Error("provided sign is wrong, but check passed")
	}
	correctSign := generateChannelSign(secretKey, client, channel, channelData)
	result = checkChannelSign(secretKey, client, channel, channelData, correctSign)
	if !result {
		t.Error("correct sign must pass check")
	}
}
