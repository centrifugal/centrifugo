package jwtverify

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cristalhq/jwt/v3"
)

type VerifierConfig struct {
	// HMACSecretKey is a secret key used to validate connection and subscription
	// tokens generated using HMAC. Zero value means that HMAC tokens won't be allowed.
	HMACSecretKey string
	// RSAPublicKey is a public key used to validate connection and subscription
	// tokens generated using RSA. Zero value means that RSA tokens won't be allowed.
	RSAPublicKey *rsa.PublicKey
}

func NewTokenVerifierJWT(config VerifierConfig) *VerifierJWT {
	verifier := &VerifierJWT{}
	algorithms, err := newAlgorithms(config.HMACSecretKey, config.RSAPublicKey)
	if err != nil {
		panic(err)
	}
	verifier.algorithms = algorithms
	return verifier
}

type VerifierJWT struct {
	mu         sync.RWMutex
	algorithms *algorithms
}

var (
	ErrTokenExpired         = errors.New("token expired")
	errUnsupportedAlgorithm = errors.New("unsupported JWT algorithm")
	errDisabledAlgorithm    = errors.New("disabled JWT algorithm")
)

type ConnectTokenClaims struct {
	Info       json.RawMessage `json:"info,omitempty"`
	Base64Info string          `json:"b64info,omitempty"`
	Channels   []string        `json:"channels,omitempty"`
	jwt.StandardClaims
}

type SubscribeTokenClaims struct {
	Client          string          `json:"client,omitempty"`
	Channel         string          `json:"channel,omitempty"`
	Info            json.RawMessage `json:"info,omitempty"`
	Base64Info      string          `json:"b64info,omitempty"`
	ExpireTokenOnly bool            `json:"eto,omitempty"`
	jwt.StandardClaims
}

type algorithms struct {
	HS256 jwt.Verifier
	HS384 jwt.Verifier
	HS512 jwt.Verifier
	RS256 jwt.Verifier
	RS384 jwt.Verifier
	RS512 jwt.Verifier
}

func newAlgorithms(tokenHMACSecretKey string, pubKey *rsa.PublicKey) (*algorithms, error) {
	alg := &algorithms{}

	// HMAC SHA.
	if tokenHMACSecretKey != "" {
		verifierHS256, err := jwt.NewVerifierHS(jwt.HS256, []byte(tokenHMACSecretKey))
		if err != nil {
			return nil, err
		}
		verifierHS384, err := jwt.NewVerifierHS(jwt.HS384, []byte(tokenHMACSecretKey))
		if err != nil {
			return nil, err
		}
		verifierHS512, err := jwt.NewVerifierHS(jwt.HS512, []byte(tokenHMACSecretKey))
		if err != nil {
			return nil, err
		}
		alg.HS256 = verifierHS256
		alg.HS384 = verifierHS384
		alg.HS512 = verifierHS512
	}

	// RSA.
	if pubKey != nil {
		verifierRS256, err := jwt.NewVerifierRS(jwt.RS256, pubKey)
		if err != nil {
			return nil, err
		}
		verifierRS384, err := jwt.NewVerifierRS(jwt.RS384, pubKey)
		if err != nil {
			return nil, err
		}
		verifierRS512, err := jwt.NewVerifierRS(jwt.RS512, pubKey)
		if err != nil {
			return nil, err
		}
		alg.RS256 = verifierRS256
		alg.RS384 = verifierRS384
		alg.RS512 = verifierRS512
	}

	return alg, nil
}

func (s *algorithms) verify(token *jwt.Token) error {
	var verifier jwt.Verifier
	switch token.Header().Algorithm {
	case jwt.HS256:
		verifier = s.HS256
	case jwt.HS384:
		verifier = s.HS384
	case jwt.HS512:
		verifier = s.HS512
	case jwt.RS256:
		verifier = s.RS256
	case jwt.RS384:
		verifier = s.RS384
	case jwt.RS512:
		verifier = s.RS512
	default:
		return fmt.Errorf("%w: %s", errUnsupportedAlgorithm, string(token.Header().Algorithm))
	}
	if verifier == nil {
		return fmt.Errorf("%w: %s", errDisabledAlgorithm, string(token.Header().Algorithm))
	}
	return verifier.Verify(token.Payload(), token.Signature())
}

func (verifier *VerifierJWT) verifySignature(token *jwt.Token) error {
	verifier.mu.RLock()
	defer verifier.mu.RUnlock()
	return verifier.algorithms.verify(token)
}

func (verifier *VerifierJWT) VerifyConnectToken(t string) (ConnectToken, error) {
	token, err := jwt.Parse([]byte(t))
	if err != nil {
		return ConnectToken{}, err
	}

	err = verifier.verifySignature(token)
	if err != nil {
		return ConnectToken{}, err
	}

	claims := &ConnectTokenClaims{}
	err = json.Unmarshal(token.RawClaims(), claims)
	if err != nil {
		return ConnectToken{}, err
	}

	now := time.Now()
	if !claims.IsValidExpiresAt(now) || !claims.IsValidNotBefore(now) {
		return ConnectToken{}, ErrTokenExpired
	}

	ct := ConnectToken{
		UserID:   claims.StandardClaims.Subject,
		Info:     claims.Info,
		Channels: claims.Channels,
	}
	if claims.ExpiresAt != nil {
		ct.ExpireAt = claims.ExpiresAt.Unix()
	}
	if claims.Base64Info != "" {
		byteInfo, err := base64.StdEncoding.DecodeString(claims.Base64Info)
		if err != nil {
			return ConnectToken{}, err
		}
		ct.Info = byteInfo
	}
	return ct, nil
}

func (verifier *VerifierJWT) VerifySubscribeToken(t string) (SubscribeToken, error) {
	token, err := jwt.Parse([]byte(t))
	if err != nil {
		return SubscribeToken{}, err
	}

	err = verifier.verifySignature(token)
	if err != nil {
		return SubscribeToken{}, err
	}

	claims := &SubscribeTokenClaims{}
	err = json.Unmarshal(token.RawClaims(), claims)
	if err != nil {
		return SubscribeToken{}, err
	}

	now := time.Now()
	if !claims.IsValidExpiresAt(now) || !claims.IsValidNotBefore(now) {
		return SubscribeToken{}, ErrTokenExpired
	}

	st := SubscribeToken{
		Client:          claims.Client,
		Info:            claims.Info,
		Channel:         claims.Channel,
		ExpireTokenOnly: claims.ExpireTokenOnly,
	}
	if claims.ExpiresAt != nil {
		st.ExpireAt = claims.ExpiresAt.Unix()
	}
	if claims.Base64Info != "" {
		byteInfo, err := base64.StdEncoding.DecodeString(claims.Base64Info)
		if err != nil {
			return SubscribeToken{}, err
		}
		st.Info = byteInfo
	}
	return st, nil
}

func (verifier *VerifierJWT) Reload(config VerifierConfig) error {
	verifier.mu.Lock()
	defer verifier.mu.Unlock()
	alg, err := newAlgorithms(config.HMACSecretKey, config.RSAPublicKey)
	if err != nil {
		return err
	}
	verifier.algorithms = alg
	return nil
}
