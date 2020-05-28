package tools

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/cristalhq/jwt/v3"
)

// GenerateToken generates sample JWT for user.
func GenerateToken(config centrifuge.Config, user string, ttlSeconds int64) (string, error) {
	if config.TokenHMACSecretKey == "" {
		return "", fmt.Errorf("no token_hmac_secret_key found in config")
	}
	signer, _ := jwt.NewSignerHS(jwt.HS256, []byte(config.TokenHMACSecretKey))
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

func verify(config centrifuge.Config, token *jwt.Token) error {
	var verifier jwt.Verifier
	errDisabled := fmt.Errorf("algorithm is not enabled in configuration file: %s", string(token.Header().Algorithm))
	switch token.Header().Algorithm {
	case jwt.HS256:
		if config.TokenHMACSecretKey == "" {
			return errDisabled
		}
		verifier, _ = jwt.NewVerifierHS(jwt.HS256, []byte(config.TokenHMACSecretKey))
	case jwt.HS384:
		if config.TokenHMACSecretKey == "" {
			return errDisabled
		}
		verifier, _ = jwt.NewVerifierHS(jwt.HS384, []byte(config.TokenHMACSecretKey))
	case jwt.HS512:
		if config.TokenHMACSecretKey == "" {
			return errDisabled
		}
		verifier, _ = jwt.NewVerifierHS(jwt.HS512, []byte(config.TokenHMACSecretKey))
	case jwt.RS256:
		if config.TokenRSAPublicKey == nil {
			return errDisabled
		}
		verifier, _ = jwt.NewVerifierRS(jwt.RS256, config.TokenRSAPublicKey)
	case jwt.RS384:
		if config.TokenRSAPublicKey == nil {
			return errDisabled
		}
		verifier, _ = jwt.NewVerifierRS(jwt.RS256, config.TokenRSAPublicKey)
	case jwt.RS512:
		if config.TokenRSAPublicKey == nil {
			return errDisabled
		}
		verifier, _ = jwt.NewVerifierRS(jwt.RS256, config.TokenRSAPublicKey)
	default:
		return fmt.Errorf("unsupported JWT algorithm: %s", string(token.Header().Algorithm))
	}
	return verifier.Verify(token.Payload(), token.Signature())
}

// CheckToken checks JWT for user.
func CheckToken(config centrifuge.Config, t string) (string, []byte, error) {
	token, err := jwt.Parse([]byte(t))
	if err != nil {
		return "", nil, err
	}
	err = verify(config, token)
	if err != nil {
		return "", nil, err
	}

	claims := &jwt.StandardClaims{}
	err = json.Unmarshal(token.RawClaims(), claims)
	if err != nil {
		return "", nil, err
	}

	now := time.Now()
	if !claims.IsValidExpiresAt(now) {
		return "", nil, fmt.Errorf("expired token for user %s", claims.Subject)
	}

	if !claims.IsValidNotBefore(now) {
		return "", nil, fmt.Errorf("token can not be used yet for user %s", claims.Subject)
	}

	return claims.Subject, token.RawClaims(), nil
}
