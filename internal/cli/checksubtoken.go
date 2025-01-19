package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/confighelpers"
	"github.com/centrifugal/centrifugo/v6/internal/jwtverify"

	"github.com/cristalhq/jwt/v5"
	"github.com/spf13/cobra"
)

func CheckSubToken() *cobra.Command {
	var checkSubTokenConfigFile string
	var checkSubTokenCmd = &cobra.Command{
		Use:   "checksubtoken [TOKEN]",
		Short: "Check subscription JWT",
		Long:  `Check subscription JWT`,
		Run: func(cmd *cobra.Command, args []string) {
			checkSubToken(cmd, checkSubTokenConfigFile, args)
		},
	}
	checkSubTokenCmd.Flags().StringVarP(&checkSubTokenConfigFile, "config", "c", "config.json", "path to config file")
	return checkSubTokenCmd
}

func checkSubToken(cmd *cobra.Command, checkSubTokenConfigFile string, args []string) {
	cfg, _, err := config.GetConfig(cmd, checkSubTokenConfigFile)
	if err != nil {
		fmt.Printf("error getting config: %v\n", err)
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
	if len(args) != 1 {
		fmt.Printf("error: provide token to check [centrifugo checksubtoken <TOKEN>]\n")
		os.Exit(1)
	}
	subject, channel, claims, err := parseAndVerifySubToken(verifierConfig, cfg, args[0])
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	var user = fmt.Sprintf("user \"%s\"", subject)
	if subject == "" {
		user = "anonymous user"
	}
	fmt.Printf("valid subscription token for %s and channel \"%s\"\npayload: %s\n", user, channel, string(claims))
}

// parseAndVerifySubToken checks subscription JWT for user.
func parseAndVerifySubToken(config jwtverify.VerifierConfig, cfg config.Config, t string) (string, string, []byte, error) {
	token, err := jwt.ParseNoVerify([]byte(t)) // Will be verified later.
	if err != nil {
		return "", "", nil, err
	}

	var claims jwtverify.SubscribeTokenClaims
	err = json.Unmarshal(token.Claims(), &claims)
	if err != nil {
		return "", "", nil, err
	}

	ct, err := verifySub(config, cfg, t)
	if err != nil {
		return "", "", nil, fmt.Errorf("token with algorithm %s and claims %s has error: %v", token.Header().Algorithm, string(token.Claims()), err)
	}

	return ct.UserID, ct.Channel, token.Claims(), nil
}

func verifySub(verifierConf jwtverify.VerifierConfig, cfg config.Config, token string) (jwtverify.SubscribeToken, error) {
	cfgContainer, err := config.NewContainer(cfg)
	if err != nil {
		return jwtverify.SubscribeToken{}, err
	}
	verifier, err := jwtverify.NewTokenVerifierJWT(verifierConf, cfgContainer)
	if err != nil {
		return jwtverify.SubscribeToken{}, err
	}
	return verifier.VerifySubscribeToken(token, false)
}
