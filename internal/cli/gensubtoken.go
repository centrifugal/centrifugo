package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/config"
	"github.com/centrifugal/centrifugo/v5/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v5/internal/runutil"

	"github.com/cristalhq/jwt/v5"
	"github.com/spf13/cobra"
	"github.com/tidwall/sjson"
)

func GenSubToken(
	cmd *cobra.Command, genSubTokenConfigFile string, genSubTokenUser string,
	genSubTokenChannel string, genSubTokenTTL int64, genSubTokenQuiet bool,
) {
	cfg, _, err := config.GetConfig(cmd, genSubTokenConfigFile)
	if err != nil {
		fmt.Printf("error getting config: %v\n", err)
		os.Exit(1)
	}
	if genSubTokenChannel == "" {
		fmt.Println("channel is required")
		os.Exit(1)
	}
	verifierConfig, err := runutil.JWTVerifierConfig(cfg)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	if cfg.Client.SubscriptionToken.Enabled {
		verifierConfig, err = runutil.SubJWTVerifierConfig(cfg)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			os.Exit(1)
		}
	}
	token, err := generateSubToken(verifierConfig, genSubTokenUser, genSubTokenChannel, genSubTokenTTL)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	var user = fmt.Sprintf("user \"%s\"", genSubTokenUser)
	if genSubTokenUser == "" {
		user = "anonymous user"
	}
	exp := "without expiration"
	if genSubTokenTTL >= 0 {
		exp = fmt.Sprintf("with expiration TTL %s", time.Duration(genSubTokenTTL)*time.Second)
	}
	if genSubTokenQuiet {
		fmt.Print(token)
		return
	}
	fmt.Printf("HMAC SHA-256 JWT for %s and channel \"%s\" %s:\n%s\n", user, genSubTokenChannel, exp, token)
}

// generateSubToken generates sample subscription JWT for user.
func generateSubToken(config jwtverify.VerifierConfig, user string, channel string, ttlSeconds int64) (string, error) {
	if config.HMACSecretKey == "" {
		return "", errors.New("no HMAC secret key set")
	}
	signer, err := jwt.NewSignerHS(jwt.HS256, []byte(config.HMACSecretKey))
	if err != nil {
		return "", fmt.Errorf("error creating HMAC signer: %w", err)
	}
	builder := jwt.NewBuilder(signer)
	claims := jwtverify.SubscribeTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(time.Now()),
			Subject:  user,
		},
		Channel: channel,
	}
	if ttlSeconds > 0 {
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
	}

	encodedClaims, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	if config.UserIDClaim != "" {
		encodedClaims, err = sjson.SetBytes(encodedClaims, config.UserIDClaim, user)
		if err != nil {
			return "", err
		}
	}

	token, err := builder.Build(encodedClaims)
	if err != nil {
		return "", err
	}
	return token.String(), nil
}
