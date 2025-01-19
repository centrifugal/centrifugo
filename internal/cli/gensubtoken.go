package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/confighelpers"
	"github.com/centrifugal/centrifugo/v6/internal/jwtverify"

	"github.com/cristalhq/jwt/v5"
	"github.com/spf13/cobra"
	"github.com/tidwall/sjson"
)

func GenSubToken() *cobra.Command {
	var genSubTokenConfigFile string
	var genSubTokenUser string
	var genSubTokenChannel string
	var genSubTokenTTL int64
	var genSubTokenQuiet bool
	var genSubTokenCmd = &cobra.Command{
		Use:   "gensubtoken",
		Short: "Generate sample subscription JWT for user",
		Long:  `Generate sample subscription JWT for user`,
		Run: func(cmd *cobra.Command, args []string) {
			genSubToken(cmd, genSubTokenConfigFile, genSubTokenUser, genSubTokenChannel, genSubTokenTTL, genSubTokenQuiet)
		},
	}
	genSubTokenCmd.Flags().StringVarP(&genSubTokenConfigFile, "config", "c", "config.json", "path to config file")
	genSubTokenCmd.Flags().StringVarP(&genSubTokenUser, "user", "u", "", "user ID")
	genSubTokenCmd.Flags().StringVarP(&genSubTokenChannel, "channel", "s", "", "channel")
	genSubTokenCmd.Flags().Int64VarP(&genSubTokenTTL, "ttl", "t", 3600*24*7, "token TTL in seconds, use -1 for token without expiration")
	genSubTokenCmd.Flags().BoolVarP(&genSubTokenQuiet, "quiet", "q", false, "only output the token without anything else")
	return genSubTokenCmd
}

func genSubToken(
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
	verifierConfig, err := confighelpers.MakeVerifierConfig(cfg.Client.Token)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	if cfg.Client.SubscriptionToken.Enabled {
		verifierConfig, err = confighelpers.MakeVerifierConfig(cfg.Client.SubscriptionToken.Token)
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
