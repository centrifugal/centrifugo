package jwtverify

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/jwks"
	"github.com/centrifugal/centrifugo/v6/internal/subsource"

	"github.com/centrifugal/centrifuge"
	"github.com/cristalhq/jwt/v5"
	"github.com/rakutentech/jwk-go/okp"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"
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

	// AudienceRegex allows setting Audience in form of Go language regex pattern. Regex groups
	// may be then used in constructing JWKSPublicEndpoint.
	AudienceRegex string

	// Issuer when set will enable a check that token issuer matches configured string.
	// See https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.1.
	Issuer string

	// IssuerRegex allows setting Issuer in form of Go language regex pattern. Regex groups
	// may be then used in constructing JWKSPublicEndpoint.
	IssuerRegex string

	// UserIDClaim allows overriding default claim used to extract user ID from token.
	// By default, Centrifugo uses "sub" and we recommend keeping the default if possible.
	UserIDClaim string
}

func (c VerifierConfig) Validate() error {
	if c.Audience != "" && c.AudienceRegex != "" {
		return errors.New("can not use both token_audience and token_audience_regex, configure only one of them")
	}
	if c.Issuer != "" && c.IssuerRegex != "" {
		return errors.New("can not use both token_issuer and token_issuer_regex, configure only one of them")
	}
	return nil
}

func NewTokenVerifierJWT(config VerifierConfig, cfgContainer *config.Container) (*VerifierJWT, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("error validating token verifier config: %w", err)
	}
	var audienceRe *regexp.Regexp
	var issuerRe *regexp.Regexp
	var err error
	if config.AudienceRegex != "" {
		audienceRe, err = regexp.Compile(config.AudienceRegex)
		if err != nil {
			return nil, fmt.Errorf("error compiling audience regex: %w", err)
		}
	}
	if config.IssuerRegex != "" {
		issuerRe, err = regexp.Compile(config.IssuerRegex)
		if err != nil {
			return nil, fmt.Errorf("error compiling issuer regex: %w", err)
		}
	}

	verifier := &VerifierJWT{
		cfgContainer: cfgContainer,
		issuer:       config.Issuer,
		issuerRe:     issuerRe,
		audience:     config.Audience,
		audienceRe:   audienceRe,
		userIDClaim:  config.UserIDClaim,
	}

	algorithms, err := newAlgorithms(config.HMACSecretKey, config.RSAPublicKey, config.ECDSAPublicKey)
	if err != nil {
		return nil, fmt.Errorf("error initializing token algorithms: %w", err)
	}
	verifier.algorithms = algorithms

	if config.JWKSPublicEndpoint != "" {
		mng, err := jwks.NewManager(config.JWKSPublicEndpoint)
		if err != nil {
			return nil, fmt.Errorf("error creating JWK manager: %w", err)
		}
		verifier.jwksManager = &jwksManager{mng}
	}

	return verifier, nil
}

type VerifierJWT struct {
	mu           sync.RWMutex
	jwksManager  *jwksManager
	algorithms   *algorithms
	cfgContainer *config.Container
	audience     string
	audienceRe   *regexp.Regexp
	issuer       string
	issuerRe     *regexp.Regexp
	userIDClaim  string
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
	// ForcePushJoinLeave forces sending join/leave for this client.
	ForcePushJoinLeave *BoolValue `json:"force_push_join_leave,omitempty"`
	// ForcePositioning on says that client will additionally sync its position inside
	// a stream to prevent message loss. Make sure you are enabling ForcePositioning in channels
	// that maintain Publication history stream. When ForcePositioning is on  Centrifuge will
	// include StreamPosition information to subscribe response - for a client to be able
	// to manually track its position inside a stream.
	ForcePositioning *BoolValue `json:"force_positioning,omitempty"`
	// ForceRecovery turns on recovery option for a channel. In this case client will try to
	// recover missed messages automatically upon resubscribe to a channel after reconnect
	// to a server. This option also enables client position tracking inside a stream
	// (like ForcePositioning option) to prevent occasional message loss. Make sure you are using
	// ForceRecovery in channels that maintain Publication history stream.
	ForceRecovery *BoolValue `json:"force_recovery,omitempty"`
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
	// Channel must never be set in connection tokens. We check this on verifying.
	Channel string `json:"channel,omitempty"`
	jwt.RegisteredClaims
}

type SubscribeTokenClaims struct {
	jwt.RegisteredClaims
	SubscribeOptions
	Channel  string `json:"channel,omitempty"`
	Client   string `json:"client,omitempty"`
	ExpireAt *int64 `json:"expire_at,omitempty"`
}

type jwksManager struct{ *jwks.Manager }

func (j *jwksManager) verify(token *jwt.Token, tokenVars map[string]any) error {
	kid := token.Header().KeyID

	key, err := j.Manager.FetchKey(context.Background(), kid, tokenVars)
	if err != nil {
		return err
	}

	if key.Kty != "RSA" && key.Kty != "EC" && key.Kty != "OKP" {
		return errUnsupportedAlgorithm
	}

	spec, err := key.ParseKeySpec()
	if err != nil {
		return fmt.Errorf("error parsing key spec: %w", err)
	}

	switch key.Kty {
	case "RSA":
		pubKey, ok := spec.Key.(*rsa.PublicKey)
		if !ok {
			return errPublicKeyInvalid
		}

		verifier, err := jwt.NewVerifierRS(jwt.Algorithm(spec.Algorithm), pubKey)
		if err != nil {
			return fmt.Errorf("%w: %s", errUnsupportedAlgorithm, spec.Algorithm)
		}

		return verifier.Verify(token)
	case "EC":
		pubKey, ok := spec.Key.(*ecdsa.PublicKey)
		if !ok {
			return errPublicKeyInvalid
		}

		verifier, err := jwt.NewVerifierES(jwt.Algorithm(spec.Algorithm), pubKey)
		if err != nil {
			return fmt.Errorf("%w: %s", errUnsupportedAlgorithm, spec.Algorithm)
		}

		return verifier.Verify(token)
	case "OKP":
		pubKey, ok := spec.Key.(okp.Ed25519)
		if !ok {
			return errPublicKeyInvalid
		}

		verifier, err := jwt.NewVerifierEdDSA(pubKey.PublicKey())
		if err != nil {
			return errUnsupportedAlgorithm
		}

		return verifier.Verify(token)
	default:
		return errUnsupportedAlgorithm
	}
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
			if !errors.Is(err, jwt.ErrInvalidKey) {
				return nil, err
			}
		} else {
			alg.RS256 = verifierRS256
			algorithms = append(algorithms, "RS256")
		}
		if verifierRS384, err := jwt.NewVerifierRS(jwt.RS384, rsaPubKey); err != nil {
			if !errors.Is(err, jwt.ErrInvalidKey) {
				return nil, err
			}
		} else {
			alg.RS384 = verifierRS384
			algorithms = append(algorithms, "RS384")
		}
		if verifierRS512, err := jwt.NewVerifierRS(jwt.RS512, rsaPubKey); err != nil {
			if !errors.Is(err, jwt.ErrInvalidKey) {
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
			if !errors.Is(err, jwt.ErrInvalidKey) {
				return nil, err
			}
		} else {
			alg.ES256 = verifierES256
			algorithms = append(algorithms, "ES256")
		}
		if verifierES384, err := jwt.NewVerifierES(jwt.ES384, ecdsaPubKey); err != nil {
			if !errors.Is(err, jwt.ErrInvalidKey) {
				return nil, err
			}
		} else {
			alg.ES384 = verifierES384
			algorithms = append(algorithms, "ES384")
		}
		if verifierES512, err := jwt.NewVerifierES(jwt.ES512, ecdsaPubKey); err != nil {
			if !errors.Is(err, jwt.ErrInvalidKey) {
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
	return verifier.Verify(token)
}

func (verifier *VerifierJWT) verifySignature(token *jwt.Token) error {
	verifier.mu.RLock()
	defer verifier.mu.RUnlock()

	return verifier.algorithms.verify(token)
}

func (verifier *VerifierJWT) verifySignatureByJWK(token *jwt.Token, tokenVars map[string]any) error {
	verifier.mu.RLock()
	defer verifier.mu.RUnlock()

	return verifier.jwksManager.verify(token, tokenVars)
}

func (verifier *VerifierJWT) VerifyConnectToken(t string, skipVerify bool) (ConnectToken, error) {
	token, err := jwt.ParseNoVerify([]byte(t)) // Will be verified later.
	if err != nil {
		return ConnectToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, err := claimsDecoder.DecodeConnectClaims(token.Claims())
	if err != nil {
		return ConnectToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if verifier.audience != "" && !claims.IsForAudience(verifier.audience) {
		return ConnectToken{}, fmt.Errorf("%w: invalid audience", ErrInvalidToken)
	}

	if verifier.issuer != "" && !claims.IsIssuer(verifier.issuer) {
		return ConnectToken{}, fmt.Errorf("%w: invalid issuer", ErrInvalidToken)
	}

	tokenVars := map[string]any{}

	if verifier.issuerRe != nil {
		match := verifier.issuerRe.FindStringSubmatch(claims.Issuer)
		if len(match) == 0 {
			return ConnectToken{}, fmt.Errorf("%w: issuer not matched", ErrInvalidToken)
		}
		for i, name := range verifier.issuerRe.SubexpNames() {
			if i != 0 && name != "" {
				tokenVars[name] = match[i]
			}
		}
	}

	if verifier.audienceRe != nil {
		matched := false
		for _, audience := range claims.Audience {
			match := verifier.audienceRe.FindStringSubmatch(audience)
			if len(match) == 0 {
				continue
			}
			matched = true
			for i, name := range verifier.audienceRe.SubexpNames() {
				if i != 0 && name != "" {
					tokenVars[name] = match[i]
				}
			}
			break
		}
		if !matched {
			return ConnectToken{}, fmt.Errorf("%w: audience not matched", ErrInvalidToken)
		}
	}

	if !skipVerify {
		if verifier.jwksManager != nil {
			err = verifier.verifySignatureByJWK(token, tokenVars)
		} else {
			err = verifier.verifySignature(token)
		}
		if err != nil {
			return ConnectToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
		}
	}

	if claims.Channel != "" {
		return ConnectToken{}, fmt.Errorf(
			"%w: connection JWT can not contain channel claim, only subscription JWT can", ErrInvalidToken)
	}

	now := time.Now()
	if !claims.IsValidExpiresAt(now) || !claims.IsValidNotBefore(now) {
		return ConnectToken{}, ErrTokenExpired
	}

	subs := map[string]centrifuge.SubscribeOptions{}

	if len(claims.Subs) > 0 {
		for ch, v := range claims.Subs {
			_, _, chOpts, found, err := verifier.cfgContainer.ChannelOptions(ch)
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
			pushJoinLeave := chOpts.ForcePushJoinLeave
			if v.Override != nil && v.Override.ForcePushJoinLeave != nil {
				pushJoinLeave = v.Override.ForcePushJoinLeave.Value
			}
			recovery := chOpts.ForceRecovery
			if v.Override != nil && v.Override.ForceRecovery != nil {
				recovery = v.Override.ForceRecovery.Value
			}
			positioning := chOpts.ForcePositioning
			if v.Override != nil && v.Override.ForcePositioning != nil {
				positioning = v.Override.ForcePositioning.Value
			}
			recoveryMode := chOpts.GetRecoveryMode()
			subs[ch] = centrifuge.SubscribeOptions{
				ChannelInfo:       info,
				EmitPresence:      presence,
				EmitJoinLeave:     joinLeave,
				PushJoinLeave:     pushJoinLeave,
				EnableRecovery:    recovery,
				EnablePositioning: positioning,
				RecoveryMode:      recoveryMode,
				Data:              data,
				Source:            subsource.ConnectionToken,
				HistoryMetaTTL:    chOpts.HistoryMetaTTL.ToDuration(),
				AllowedDeltaTypes: chOpts.AllowedDeltaTypes,
			}
		}
	} else if len(claims.Channels) > 0 {
		for _, ch := range claims.Channels {
			_, _, chOpts, found, err := verifier.cfgContainer.ChannelOptions(ch)
			if err != nil {
				return ConnectToken{}, err
			}
			if !found {
				return ConnectToken{}, centrifuge.ErrorUnknownChannel
			}
			subs[ch] = centrifuge.SubscribeOptions{
				EmitPresence:      chOpts.Presence,
				EmitJoinLeave:     chOpts.JoinLeave,
				PushJoinLeave:     chOpts.ForcePushJoinLeave,
				EnableRecovery:    chOpts.ForceRecovery,
				EnablePositioning: chOpts.ForcePositioning,
				RecoveryMode:      chOpts.GetRecoveryMode(),
				Source:            subsource.ConnectionToken,
				HistoryMetaTTL:    chOpts.HistoryMetaTTL.ToDuration(),
				AllowedDeltaTypes: chOpts.AllowedDeltaTypes,
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
		Info:     info,
		Subs:     subs,
		ExpireAt: expireAt,
		Meta:     claims.Meta,
	}
	if verifier.userIDClaim != "" {
		value := gjson.GetBytes(token.Claims(), verifier.userIDClaim)
		ct.UserID = value.String()
	} else {
		ct.UserID = claims.RegisteredClaims.Subject
	}
	return ct, nil
}

func (verifier *VerifierJWT) VerifySubscribeToken(t string, skipVerify bool) (SubscribeToken, error) {
	token, err := jwt.ParseNoVerify([]byte(t)) // Will be verified later.
	if err != nil {
		return SubscribeToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, err := claimsDecoder.DecodeSubscribeClaims(token.Claims())
	if err != nil {
		return SubscribeToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if verifier.audience != "" && !claims.IsForAudience(verifier.audience) {
		return SubscribeToken{}, fmt.Errorf("%w: invalid audience", ErrInvalidToken)
	}

	if verifier.issuer != "" && !claims.IsIssuer(verifier.issuer) {
		return SubscribeToken{}, fmt.Errorf("%w: invalid issuer", ErrInvalidToken)
	}

	tokenVars := map[string]any{}

	if verifier.issuerRe != nil {
		match := verifier.issuerRe.FindStringSubmatch(claims.Issuer)
		if len(match) == 0 {
			return SubscribeToken{}, fmt.Errorf("%w: issuer not matched", ErrInvalidToken)
		}
		for i, name := range verifier.issuerRe.SubexpNames() {
			if i != 0 && name != "" {
				tokenVars[name] = match[i]
			}
		}
	}

	if verifier.audienceRe != nil {
		matched := false
		for _, audience := range claims.Audience {
			match := verifier.audienceRe.FindStringSubmatch(audience)
			if len(match) == 0 {
				continue
			}
			matched = true
			for i, name := range verifier.audienceRe.SubexpNames() {
				if i != 0 && name != "" {
					tokenVars[name] = match[i]
				}
			}
			break
		}
		if !matched {
			return SubscribeToken{}, fmt.Errorf("%w: audience not matched", ErrInvalidToken)
		}
	}

	if !skipVerify {
		if verifier.jwksManager != nil {
			err = verifier.verifySignatureByJWK(token, tokenVars)
		} else {
			err = verifier.verifySignature(token)
		}
		if err != nil {
			return SubscribeToken{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
		}
	}

	now := time.Now()
	if !claims.IsValidExpiresAt(now) || !claims.IsValidNotBefore(now) {
		return SubscribeToken{}, ErrTokenExpired
	}

	if claims.Channel == "" {
		return SubscribeToken{}, fmt.Errorf("%w: channel claim is required for subscription JWT", ErrInvalidToken)
	}

	_, _, chOpts, found, err := verifier.cfgContainer.ChannelOptions(claims.Channel)
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
	pushJoinLeave := chOpts.ForcePushJoinLeave
	if claims.Override != nil && claims.Override.ForcePushJoinLeave != nil {
		pushJoinLeave = claims.Override.ForcePushJoinLeave.Value
	}
	recovery := chOpts.ForceRecovery
	if claims.Override != nil && claims.Override.ForceRecovery != nil {
		recovery = claims.Override.ForceRecovery.Value
	}
	positioning := chOpts.ForcePositioning
	if claims.Override != nil && claims.Override.ForcePositioning != nil {
		positioning = claims.Override.ForcePositioning.Value
	}
	recoveryMode := chOpts.GetRecoveryMode()

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
		UserID:  claims.RegisteredClaims.Subject,
		Channel: claims.Channel,
		Client:  claims.Client,
		Options: centrifuge.SubscribeOptions{
			ExpireAt:          expireAt,
			ChannelInfo:       info,
			EmitPresence:      presence,
			EmitJoinLeave:     joinLeave,
			PushJoinLeave:     pushJoinLeave,
			EnableRecovery:    recovery,
			EnablePositioning: positioning,
			RecoveryMode:      recoveryMode,
			AllowedDeltaTypes: chOpts.AllowedDeltaTypes,
			Data:              data,
		},
	}
	if verifier.userIDClaim != "" {
		value := gjson.GetBytes(token.Claims(), verifier.userIDClaim)
		st.UserID = value.String()
	} else {
		st.UserID = claims.RegisteredClaims.Subject
	}
	return st, nil
}

func (verifier *VerifierJWT) Reload(config VerifierConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("error validating token verifier config: %w", err)
	}

	verifier.mu.Lock()
	defer verifier.mu.Unlock()

	alg, err := newAlgorithms(config.HMACSecretKey, config.RSAPublicKey, config.ECDSAPublicKey)
	if err != nil {
		return err
	}

	var audienceRe *regexp.Regexp
	var issuerRe *regexp.Regexp
	if config.AudienceRegex != "" {
		audienceRe, err = regexp.Compile(config.AudienceRegex)
		if err != nil {
			return fmt.Errorf("error compiling audience regex: %w", err)
		}
	}
	if config.IssuerRegex != "" {
		issuerRe, err = regexp.Compile(config.IssuerRegex)
		if err != nil {
			return fmt.Errorf("error compiling issuer regex: %w", err)
		}
	}
	verifier.algorithms = alg
	verifier.audience = config.Audience
	verifier.audienceRe = audienceRe
	verifier.issuer = config.Issuer
	verifier.issuerRe = issuerRe
	verifier.userIDClaim = config.UserIDClaim
	return nil
}
