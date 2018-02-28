package auth

import (
	"testing"
)

func TestGenerateClientSign(t *testing.T) {
	var (
		secretKey = "secret"
		user      = "user"
		exp       = "1430669930"
		info      = "{}"
		opts      = ""
	)
	tokenWithInfo := GenerateClientSign(secretKey, user, exp, info, opts)
	if len(tokenWithInfo) != 64 {
		t.Error("sha256 token length must be 64")
	}
}

func TestCheckClientSign(t *testing.T) {
	var (
		secretKey     = "secret"
		user          = "user"
		exp           = "1430669930"
		info          = "{}"
		opts          = ""
		providedToken = "token"
	)
	result := CheckClientSign(secretKey, user, exp, info, opts, providedToken)
	if result {
		t.Error("provided token is wrong, but check passed")
	}
	correctToken := GenerateClientSign(secretKey, user, exp, info, opts)
	result = CheckClientSign(secretKey, user, exp, info, opts, correctToken)
	if !result {
		t.Error("correct client token must pass check")
	}
}

func TestGenerateChannelSign(t *testing.T) {
	var (
		secretKey   = "secret"
		client      = "client"
		channel     = "channel"
		channelData = "{}"
	)
	sign := GenerateChannelSign(secretKey, client, channel, channelData)
	if len(sign) != 64 {
		t.Error("sha256 sign length must be 64")
	}
}

func TestCheckChannelSign(t *testing.T) {
	var (
		secretKey    = "secret"
		client       = "client"
		channel      = "channel"
		channelData  = "{}"
		providedSign = "sign"
	)
	result := CheckChannelSign(secretKey, client, channel, channelData, providedSign)
	if result {
		t.Error("provided sign is wrong, but check passed")
	}
	correctSign := GenerateChannelSign(secretKey, client, channel, channelData)
	result = CheckChannelSign(secretKey, client, channel, channelData, correctSign)
	if !result {
		t.Error("correct sign must pass check")
	}
}
