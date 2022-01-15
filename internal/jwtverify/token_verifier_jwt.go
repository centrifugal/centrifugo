package jwtverify

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/jwks"
	"github.com/centrifugal/centrifugo/v3/internal/rule"

	"github.com/centrifugal/centrifuge"
	"github.com/cristalhq/jwt/v3"
	"github.com/rs/zerolog/log"
)

type VerifierConfig struct {
	// HMACSecretKey is a secret key used to validate connection and subscription
	// tokens generated using HMAC. Zero value means that HMAC tokens won't be allowed.
	HMACSecretKey string

	// RSAPublicKey is a public key used to validate connection and subscription
	// tokens generated using RSA. Zero value means that RSA tokens won't be allowed.
	RSAPublicKey *rsa.PublicKey

	// ECDSAPublicKey is a public key used to validate connection and subscription
	// tokens generated using ECDSA. Zero value means that ECDSA tokens won't be allowed.
	ECDSAPublicKey *ecdsa.PublicKey

	// JWKSPublicEndpoint is a public url used to validate connection and subscription
	// tokens generated using rotating RSA public keys. Zero value means that JSON Web Key Sets
	// extension won't be used.
	JWKSPublicEndpoint string

	// Audience when set will enable audience token check. See
	// https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.3.
	Audience string

	// Issuer when set will enable a check that token issuer matches configured string.
	// See https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.1.
	Issuer string
}

func NewTokenVerifierJWT(config VerifierConfig, ruleContainer *rule.Container) *VerifierJWT {
	verifier := &VerifierJWT{
		ruleContainer: ruleContainer,
		issuer:        config.Issuer,
		audience:      config.Audience,
	}

	algorithms, err := newAlgorithms(config.HMACSecretKey, config.RSAPublicKey, config.ECDSAPublicKey)
	if err != nil {
		panic(err)
	}
	verifier.algorithms = algorithms

	if config.JWKSPublicEndpoint != "" {
		mng, err := jwks.NewManager(config.JWKSPublicEndpoint)
		if err == nil {
			verifier.jwksManager = &jwksManager{mng}
		}
	}

	return verifier
}

type VerifierJWT struct {
	mu            sync.RWMutex
	jwksManager   *jwksManager
	algorithms    *algorithms
	ruleContainer *rule.Container
	audience      string
	issuer        string
}

var (
	ErrTokenExpired         = errors.New("token expired")
	ErrInvalidToken         = errors.New("invalid token")
	errPublicKeyInvalid     = errors.New("public key is invalid")
	errUnsupportedAlgorithm = errors.New("unsupported JWT algorithm")
	errDisabledAlgorithm    = errors.New("disabled JWT algorithm")
)

// BoolValue allows override boolean option.
type BoolValue struct {
	Value bool `json:"value,omitempty"`
}

// SubscribeOptionOverride to override configured behaviour of subscriptions.
type SubscribeOptionOverride struct {
	// Presence turns on participating in channel presence.
	Presence *BoolValue `json:"presence,omitempty"`
	// JoinLeave enables sending Join and Leave messages for this client in channel.
	JoinLeave *BoolValue `json:"join_leave,omitempty"`
	// Position on says that client will additionally sync its position inside
	// a stream to prevent message loss. Make sure you are enabling Position in channels
	// that maintain Publication history stream. When Position is on  Centrifuge will
	// include StreamPosition information to subscribe response - for a client to be able
	// to manually track its position inside a stream.
	Position *BoolValue `json:"position,omitempty"`
	// Recover turns on recovery option for a channel. In this case client will try to
	// recover missed messages automatically upon resubscribe to a channel after reconnect
	// to a server. This option also enables client position tracking inside a stream
	// (like Position option) to prevent occasional message loss. Make sure you are using
	// Recover in channels that maintain Publication history stream.
	Recover *BoolValue `json:"recover,omitempty"`
}

// SubscribeOptions define per-subscription options.
type SubscribeOptions struct {
	// Info defines custom channel information, zero value means no channel information.
	Info json.RawMessage `json:"info,omitempty"`
	// Base64Info is like Info but for binary.
	Base64Info string `json:"b64info,omitempty"`
	// Data to send to a client with Subscribe Push.
	Data json.RawMessage `json:"data,omitempty"`
	// Base64Data is like Data but for binary data.
	Base64Data string `json:"b64data,omitempty"`
	// Override channel options can contain channel options overrides.
	Override *SubscribeOptionOverride `json:"override,omitempty"`
}

type ConnectTokenClaims struct {
	ExpireAt   *int64                      `json:"expire_at,omitempty"`
	Info       json.RawMessage             `json:"info,omitempty"`
	Base64Info string                      `json:"b64info,omitempty"`
	Channels   []string                    `json:"channels,omitempty"`
	Subs       map[string]SubscribeOptions `json:"subs,omitempty"`
	Meta       json.RawMessage             `json:"meta,omitempty"`
	jwt.StandardClaims
}

type SubscribeTokenClaims struct {
	jwt.StandardClaims
	SubscribeOptions
	Client   string `json:"client,omitempty"`
	Channel  string `json:"channel,omitempty"`
	ExpireAt *int64 `json:"expire_at,omitempty"`
}

type jwksManager struct{ *jwks.Manager }

func (j *jwksManager) verify(token *jwt.Token) error {
	kid := token.Header().KeyID

	key, err := j.Manager.FetchKey(context.Background(), kid)
	if err != nil {
		return err
	}

	if key.Kty != "RSA" {
		return errUnsupportedAlgorithm
	}

	spec, err := key.ParseKeySpec()
	if err != nil {
		return err
	}

	pubKey, ok := spec.Key.(*rsa.PublicKey)
	if !ok {
		return errPublicKeyInvalid
	}

	verifier, err := jwt.NewVerifierRS(jwt.Algorithm(spec.Algorithm), pubKey)
	if err != nil {
		return fmt.Errorf("%w: %s", errUnsupportedAlgorithm, spec.Algorithm)
	}

	return verifier.Verify(token.Payload(), token.Signature())
}

type algorithms struct {
	HS256 jwt.Verifier
	HS384 jwt.Verifier
	HS512 jwt.Verifier
	RS256 jwt.Verifier
	RS384 jwt.Verifier
	RS512 jwt.Verifier
	ES256 jwt.Verifier
	ES384 jwt.Verifier
	ES512 jwt.Verifier
}

func newAlgorithms(tokenHMACSecretKey string, rsaPubKey *rsa.PublicKey, ecdsaPubKey *ecdsa.PublicKey) (*algorithms, error) {
	alg := &algorithms{}

	var algorithms []string

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
		algorithms = append(algorithms, []string{"HS256", "HS384", "HS512"}...)
	}

	// RSA.
	if rsaPubKey != nil {
		if verifierRS256, err := jwt.NewVerifierRS(jwt.RS256, rsaPubKey); err != nil {
			if err != jwt.ErrInvalidKey {
				return nil, err
			}
		} else {
			alg.RS256 = verifierRS256
			algorithms = append(algorithms, "RS256")
		}
		if verifierRS384, err := jwt.NewVerifierRS(jwt.RS384, rsaPubKey); err != nil {
			if err != jwt.ErrInvalidKey {
				return nil, err
			}
		} else {
			alg.RS384 = verifierRS384
			algorithms = append(algorithms, "RS384")
		}
		if verifierRS512, err := jwt.NewVerifierRS(jwt.RS512, rsaPubKey); err != nil {
			if err != jwt.ErrInvalidKey {
				return nil, err
			}
		} else {
			alg.RS512 = verifierRS512
			algorithms = append(algorithms, "RS512")
		}
	}

	// ECDSA.
	if ecdsaPubKey != nil {
		if verifierES256, err := jwt.NewVerifierES(jwt.ES256, ecdsaPubKey); err != nil {
			if err != jwt.ErrInvalidKey {
				return nil, err
			}
		} else {
			alg.ES256 = verifierES256
			algorithms = append(algorithms, "ES256")
		}
		if verifierES384, err := jwt.NewVerifierES(jwt.ES384, ecdsaPubKey); err != nil {
			if err != jwt.ErrInvalidKey {
				return nil, err
			}
		} else {
			alg.ES384 = verifierES384
			algorithms = append(algorithms, "ES384")
		}
		if verifierES512, err := jwt.NewVerifierES(jwt.ES512, ecdsaPubKey); err != nil {
			if err != jwt.ErrInvalidKey {
				return nil, err
			}
		} else {
			alg.ES512 = verifierES512
			algorithms = append(algorithms, "ES512")
		}
	}

	if len(algorithms) > 0 {
		log.Info().Str("algorithms", strings.Join(algorithms, ", ")).Msg("enabled JWT verifiers")
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
	case jwt.ES256:
		verifier = s.ES256
	case jwt.ES384:
		verifier = s.ES384
	case jwt.ES512:
		verifier = s.ES512
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

func (verifier *VerifierJWT) verifySignatureByJWK(token *jwt.Token) error {
	verifier.mu.RLock()
	defer verifier.mu.RUnlock()

	return verifier.jwksManager.verify(token)
}

func (verifier *VerifierJWT) VerifyConnectToken(t string) (ConnectToken, error) {
	token, err := jwt.Parse([]byte(t))
	if err != nil {
		return ConnectToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if verifier.jwksManager != nil {
		err = verifier.verifySignatureByJWK(token)
	} else {
		err = verifier.verifySignature(token)
	}

	if err != nil {
		return ConnectToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, err := claimsDecoder.DecodeConnectClaims(token.RawClaims())
	if err != nil {
		return ConnectToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	now := time.Now()
	if !claims.IsValidExpiresAt(now) || !claims.IsValidNotBefore(now) {
		return ConnectToken{}, ErrTokenExpired
	}

	if verifier.audience != "" && !claims.IsForAudience(verifier.audience) {
		return ConnectToken{}, ErrInvalidToken
	}

	if verifier.issuer != "" && !claims.IsIssuer(verifier.issuer) {
		return ConnectToken{}, ErrInvalidToken
	}

	subs := map[string]centrifuge.SubscribeOptions{}

	if len(claims.Subs) > 0 {
		for ch, v := range claims.Subs {
			chOpts, found, err := verifier.ruleContainer.ChannelOptions(ch)
			if err != nil {
				return ConnectToken{}, err
			}
			if !found {
				return ConnectToken{}, centrifuge.ErrorUnknownChannel
			}
			var info []byte
			if v.Base64Info != "" {
				byteInfo, err := base64.StdEncoding.DecodeString(v.Base64Info)
				if err != nil {
					return ConnectToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
				}
				info = byteInfo
			} else {
				info = v.Info
			}
			var data []byte
			if v.Base64Data != "" {
				byteInfo, err := base64.StdEncoding.DecodeString(v.Base64Data)
				if err != nil {
					return ConnectToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
				}
				data = byteInfo
			} else {
				data = v.Data
			}
			presence := chOpts.Presence
			if v.Override != nil && v.Override.Presence != nil {
				presence = v.Override.Presence.Value
			}
			joinLeave := chOpts.JoinLeave
			if v.Override != nil && v.Override.JoinLeave != nil {
				joinLeave = v.Override.JoinLeave.Value
			}
			useRecover := chOpts.Recover
			if v.Override != nil && v.Override.Recover != nil {
				useRecover = v.Override.Recover.Value
			}
			position := chOpts.Position
			if v.Override != nil && v.Override.Position != nil {
				position = v.Override.Position.Value
			}
			subs[ch] = centrifuge.SubscribeOptions{
				ChannelInfo: info,
				Presence:    presence,
				JoinLeave:   joinLeave,
				Recover:     useRecover,
				Position:    position,
				Data:        data,
			}
		}
	} else if len(claims.Channels) > 0 {
		for _, ch := range claims.Channels {
			chOpts, found, err := verifier.ruleContainer.ChannelOptions(ch)
			if err != nil {
				return ConnectToken{}, err
			}
			if !found {
				return ConnectToken{}, centrifuge.ErrorUnknownChannel
			}
			subs[ch] = centrifuge.SubscribeOptions{
				Presence:  chOpts.Presence,
				JoinLeave: chOpts.JoinLeave,
				Recover:   chOpts.Recover,
				Position:  chOpts.Position,
			}
		}
	}

	var expireAt int64
	if claims.ExpireAt != nil {
		if *claims.ExpireAt > 0 {
			expireAt = *claims.ExpireAt
		}
	} else {
		if claims.ExpiresAt != nil {
			expireAt = claims.ExpiresAt.Unix()
		}
	}

	var info []byte
	if claims.Base64Info != "" {
		byteInfo, err := base64.StdEncoding.DecodeString(claims.Base64Info)
		if err != nil {
			return ConnectToken{}, err
		}
		info = byteInfo
	} else {
		info = claims.Info
	}

	ct := ConnectToken{
		UserID:   claims.StandardClaims.Subject,
		Info:     info,
		Subs:     subs,
		ExpireAt: expireAt,
		Meta:     claims.Meta,
	}

	return ct, nil
}

func (verifier *VerifierJWT) VerifySubscribeToken(t string) (SubscribeToken, error) {
	token, err := jwt.Parse([]byte(t))
	if err != nil {
		return SubscribeToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if verifier.jwksManager != nil {
		err = verifier.verifySignatureByJWK(token)
	} else {
		err = verifier.verifySignature(token)
	}
	if err != nil {
		return SubscribeToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, err := claimsDecoder.DecodeSubscribeClaims(token.RawClaims())
	if err != nil {
		return SubscribeToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	now := time.Now()
	if !claims.IsValidExpiresAt(now) || !claims.IsValidNotBefore(now) {
		return SubscribeToken{}, ErrTokenExpired
	}

	if verifier.audience != "" && !claims.IsForAudience(verifier.audience) {
		return SubscribeToken{}, ErrInvalidToken
	}

	if verifier.issuer != "" && !claims.IsIssuer(verifier.issuer) {
		return SubscribeToken{}, ErrInvalidToken
	}

	chOpts, found, err := verifier.ruleContainer.ChannelOptions(claims.Channel)
	if err != nil {
		return SubscribeToken{}, err
	}
	if !found {
		return SubscribeToken{}, centrifuge.ErrorUnknownChannel
	}
	var info []byte
	if claims.Base64Info != "" {
		byteInfo, err := base64.StdEncoding.DecodeString(claims.Base64Info)
		if err != nil {
			return SubscribeToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
		}
		info = byteInfo
	} else {
		info = claims.Info
	}
	var data []byte
	if claims.Base64Data != "" {
		byteInfo, err := base64.StdEncoding.DecodeString(claims.Base64Data)
		if err != nil {
			return SubscribeToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
		}
		data = byteInfo
	} else {
		data = claims.Data
	}
	presence := chOpts.Presence
	if claims.Override != nil && claims.Override.Presence != nil {
		presence = claims.Override.Presence.Value
	}
	joinLeave := chOpts.JoinLeave
	if claims.Override != nil && claims.Override.JoinLeave != nil {
		joinLeave = claims.Override.JoinLeave.Value
	}
	useRecover := chOpts.Recover
	if claims.Override != nil && claims.Override.Recover != nil {
		useRecover = claims.Override.Recover.Value
	}
	position := chOpts.Position
	if claims.Override != nil && claims.Override.Position != nil {
		position = claims.Override.Position.Value
	}

	var expireAt int64
	if claims.ExpireAt != nil {
		if *claims.ExpireAt > 0 {
			expireAt = *claims.ExpireAt
		}
	} else {
		if claims.ExpiresAt != nil {
			expireAt = claims.ExpiresAt.Unix()
		}
	}

	st := SubscribeToken{
		Client:  claims.Client,
		Channel: claims.Channel,
		Options: centrifuge.SubscribeOptions{
			ExpireAt:    expireAt,
			ChannelInfo: info,
			Presence:    presence,
			JoinLeave:   joinLeave,
			Recover:     useRecover,
			Position:    position,
			Data:        data,
		},
	}
	return st, nil
}

func (verifier *VerifierJWT) Reload(config VerifierConfig) error {
	verifier.mu.Lock()
	defer verifier.mu.Unlock()
	alg, err := newAlgorithms(config.HMACSecretKey, config.RSAPublicKey, config.ECDSAPublicKey)
	if err != nil {
		return err
	}
	verifier.algorithms = alg
	verifier.audience = config.Audience
	verifier.issuer = config.Issuer
	return nil
}
