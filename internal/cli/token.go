package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v3/internal/rule"
	"github.com/cristalhq/jwt/v3"
)

// GenerateToken generates sample JWT for user.
func GenerateToken(config jwtverify.VerifierConfig, user string, ttlSeconds int64) (string, error) {
	if config.HMACSecretKey == "" {
		return "", fmt.Errorf("no HMAC secret key set")
	}
	signer, _ := jwt.NewSignerHS(jwt.HS256, []byte(config.HMACSecretKey))
	builder := jwt.NewBuilder(signer)
	token, err := builder.Build(jwt.StandardClaims{
		Subject:   user,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(ttlSeconds) * time.Second)),
	})
	if err != nil {
		return "", err
	}
	return token.String(), nil
}

func verify(config jwtverify.VerifierConfig, ruleConfig rule.Config, token string) (jwtverify.ConnectToken, error) {
	ruleContainer := rule.NewContainer(ruleConfig)
	verifier := jwtverify.NewTokenVerifierJWT(config, ruleContainer)
	return verifier.VerifyConnectToken(token)
}

// CheckToken checks JWT for user.
func CheckToken(config jwtverify.VerifierConfig, ruleConfig rule.Config, t string) (string, []byte, error) {
	token, err := jwt.Parse([]byte(t))
	if err != nil {
		return "", nil, err
	}

	claims := &jwt.StandardClaims{}
	err = json.Unmarshal(token.RawClaims(), claims)
	if err != nil {
		return "", nil, err
	}

	ct, err := verify(config, ruleConfig, t)
	if err != nil {
		return "", nil, fmt.Errorf("token with algorithm %s and claims %s has error: %v", token.Header().Algorithm, string(token.RawClaims()), err)
	}

	return ct.UserID, token.RawClaims(), nil
}
