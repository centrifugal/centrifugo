package confighelpers

import (
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/jwtutils"
	"github.com/centrifugal/centrifugo/v6/internal/jwtverify"
)

func MakeVerifierConfig(tokenConf configtypes.Token) (jwtverify.VerifierConfig, error) {
	cfg := jwtverify.VerifierConfig{}

	cfg.HMACSecretKey = tokenConf.HMACSecretKey

	rsaPublicKey := tokenConf.RSAPublicKey
	if rsaPublicKey != "" {
		pubKey, err := jwtutils.ParseRSAPublicKeyFromPEM([]byte(rsaPublicKey))
		if err != nil {
			return jwtverify.VerifierConfig{}, fmt.Errorf("error parsing RSA public key: %w", err)
		}
		cfg.RSAPublicKey = pubKey
	}

	ecdsaPublicKey := tokenConf.ECDSAPublicKey
	if ecdsaPublicKey != "" {
		pubKey, err := jwtutils.ParseECDSAPublicKeyFromPEM([]byte(ecdsaPublicKey))
		if err != nil {
			return jwtverify.VerifierConfig{}, fmt.Errorf("error parsing ECDSA public key: %w", err)
		}
		cfg.ECDSAPublicKey = pubKey
	}

	cfg.JWKSPublicEndpoint = tokenConf.JWKSPublicEndpoint
	cfg.Audience = tokenConf.Audience
	cfg.AudienceRegex = tokenConf.AudienceRegex
	cfg.Issuer = tokenConf.Issuer
	cfg.IssuerRegex = tokenConf.IssuerRegex

	if tokenConf.UserIDClaim != "" {
		cfg.UserIDClaim = tokenConf.UserIDClaim
	}

	return cfg, nil
}
