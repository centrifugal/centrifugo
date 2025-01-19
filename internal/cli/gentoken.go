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

func GenToken() *cobra.Command {
	var genTokenConfigFile string
	var genTokenUser string
	var genTokenTTL int64
	var genTokenQuiet bool
	var genTokenCmd = &cobra.Command{
		Use:   "gentoken",
		Short: "Generate sample connection JWT for user",
		Long:  `Generate sample connection JWT for user`,
		Run: func(cmd *cobra.Command, args []string) {
			genToken(cmd, genTokenConfigFile, genTokenUser, genTokenTTL, genTokenQuiet)
		},
	}
	genTokenCmd.Flags().StringVarP(&genTokenConfigFile, "config", "c", "config.json", "path to config file")
	genTokenCmd.Flags().StringVarP(&genTokenUser, "user", "u", "", "user ID, by default anonymous")
	genTokenCmd.Flags().Int64VarP(&genTokenTTL, "ttl", "t", 3600*24*7, "token TTL in seconds, use -1 for token without expiration")
	genTokenCmd.Flags().BoolVarP(&genTokenQuiet, "quiet", "q", false, "only output the token without anything else")
	return genTokenCmd
}

func genToken(cmd *cobra.Command, genTokenConfigFile string, genTokenUser string, genTokenTTL int64, genTokenQuiet bool) {
	cfg, _, err := config.GetConfig(cmd, genTokenConfigFile)
	if err != nil {
		fmt.Printf("error getting config: %v\n", err)
		os.Exit(1)
	}
	verifierConfig, err := confighelpers.MakeVerifierConfig(cfg.Client.Token)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	token, err := generateToken(verifierConfig, genTokenUser, genTokenTTL)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	var user = fmt.Sprintf("user \"%s\"", genTokenUser)
	if genTokenUser == "" {
		user = "anonymous user"
	}
	exp := "without expiration"
	if genTokenTTL >= 0 {
		exp = fmt.Sprintf("with expiration TTL %s", time.Duration(genTokenTTL)*time.Second)
	}
	if genTokenQuiet {
		fmt.Print(token)
		return
	}
	fmt.Printf("HMAC SHA-256 JWT for %s %s:\n%s\n", user, exp, token)
}

// generateToken generates sample JWT for user.
func generateToken(config jwtverify.VerifierConfig, user string, ttlSeconds int64) (string, error) {
	if config.HMACSecretKey == "" {
		return "", errors.New("no HMAC secret key set")
	}
	signer, err := jwt.NewSignerHS(jwt.HS256, []byte(config.HMACSecretKey))
	if err != nil {
		return "", fmt.Errorf("error creating HMAC signer: %w", err)
	}
	builder := jwt.NewBuilder(signer)
	claims := jwtverify.ConnectTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(time.Now()),
			Subject:  user,
		},
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
