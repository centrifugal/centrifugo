package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v5/internal/rule"
	"github.com/cristalhq/jwt/v5"
)

// GenerateToken generates sample JWT for user.
func GenerateToken(config jwtverify.VerifierConfig, user string, ttlSeconds int64) (string, error) {
	if config.HMACSecretKey == "" {
		return "", errors.New("no HMAC secret key set")
	}
	signer, err := jwt.NewSignerHS(jwt.HS256, []byte(config.HMACSecretKey))
	if err != nil {
		return "", fmt.Errorf("error creating HMAC signer: %w", err)
	}
	builder := jwt.NewBuilder(signer)
	token, err := builder.Build(jwt.RegisteredClaims{
		Subject:   user,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(ttlSeconds) * time.Second)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	if err != nil {
		return "", err
	}
	return token.String(), nil
}

// GenerateSubToken generates sample subscription JWT for user.
func GenerateSubToken(config jwtverify.VerifierConfig, user string, channel string, ttlSeconds int64) (string, error) {
	if config.HMACSecretKey == "" {
		return "", errors.New("no HMAC secret key set")
	}
	signer, err := jwt.NewSignerHS(jwt.HS256, []byte(config.HMACSecretKey))
	if err != nil {
		return "", fmt.Errorf("error creating HMAC signer: %w", err)
	}
	builder := jwt.NewBuilder(signer)
	token, err := builder.Build(
		jwtverify.SubscribeTokenClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   user,
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(ttlSeconds) * time.Second)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
			Channel: channel,
		},
	)
	if err != nil {
		return "", err
	}
	return token.String(), nil
}

func verify(config jwtverify.VerifierConfig, ruleConfig rule.Config, token string) (jwtverify.ConnectToken, error) {
	ruleContainer, err := rule.NewContainer(ruleConfig)
	if err != nil {
		return jwtverify.ConnectToken{}, err
	}
	verifier, err := jwtverify.NewTokenVerifierJWT(config, ruleContainer)
	if err != nil {
		return jwtverify.ConnectToken{}, err
	}
	return verifier.VerifyConnectToken(token)
}

func verifySub(config jwtverify.VerifierConfig, ruleConfig rule.Config, token string) (jwtverify.SubscribeToken, error) {
	ruleContainer, err := rule.NewContainer(ruleConfig)
	if err != nil {
		return jwtverify.SubscribeToken{}, err
	}
	verifier, err := jwtverify.NewTokenVerifierJWT(config, ruleContainer)
	if err != nil {
		return jwtverify.SubscribeToken{}, err
	}
	return verifier.VerifySubscribeToken(token)
}

// CheckToken checks JWT for user.
func CheckToken(config jwtverify.VerifierConfig, ruleConfig rule.Config, t string) (string, []byte, error) {
	token, err := jwt.ParseNoVerify([]byte(t)) // Will be verified later.
	if err != nil {
		return "", nil, err
	}

	claims := &jwt.RegisteredClaims{}
	err = json.Unmarshal(token.Claims(), claims)
	if err != nil {
		return "", nil, err
	}

	ct, err := verify(config, ruleConfig, t)
	if err != nil {
		return "", nil, fmt.Errorf("token with algorithm %s and claims %s has error: %v", token.Header().Algorithm, string(token.Claims()), err)
	}

	return ct.UserID, token.Claims(), nil
}

// CheckSubToken checks subscription JWT for user.
func CheckSubToken(config jwtverify.VerifierConfig, ruleConfig rule.Config, t string) (string, string, []byte, error) {
	token, err := jwt.ParseNoVerify([]byte(t)) // Will be verified later.
	if err != nil {
		return "", "", nil, err
	}

	var claims jwtverify.SubscribeTokenClaims
	err = json.Unmarshal(token.Claims(), &claims)
	if err != nil {
		return "", "", nil, err
	}

	ct, err := verifySub(config, ruleConfig, t)
	if err != nil {
		return "", "", nil, fmt.Errorf("token with algorithm %s and claims %s has error: %v", token.Header().Algorithm, string(token.Claims()), err)
	}

	return ct.UserID, ct.Channel, token.Claims(), nil
}
