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

func CheckToken() *cobra.Command {
	var checkTokenConfigFile string
	var checkTokenCmd = &cobra.Command{
		Use:   "checktoken [TOKEN]",
		Short: "Check connection JWT",
		Long:  `Check connection JWT`,
		Run: func(cmd *cobra.Command, args []string) {
			checkToken(cmd, checkTokenConfigFile, args)
		},
	}
	checkTokenCmd.Flags().StringVarP(&checkTokenConfigFile, "config", "c", "config.json", "path to config file")
	return checkTokenCmd
}

func checkToken(cmd *cobra.Command, checkTokenConfigFile string, args []string) {
	cfg, _, err := config.GetConfig(cmd, checkTokenConfigFile)
	if err != nil {
		fmt.Printf("error getting config: %v\n", err)
		os.Exit(1)
	}
	verifierConfig, err := confighelpers.MakeVerifierConfig(cfg.Client.Token)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	if len(args) != 1 {
		fmt.Printf("error: provide token to check [centrifugo checktoken <TOKEN>]\n")
		os.Exit(1)
	}
	subject, claims, err := parseAndVerifyToken(verifierConfig, cfg, args[0])
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	var user = fmt.Sprintf("user %s", subject)
	if subject == "" {
		user = "anonymous user"
	}
	fmt.Printf("valid token for %s\npayload: %s\n", user, string(claims))
}

// parseAndVerifyToken checks JWT for user.
func parseAndVerifyToken(config jwtverify.VerifierConfig, ruleConfig config.Config, t string) (string, []byte, error) {
	token, err := jwt.ParseNoVerify([]byte(t)) // Will be verified later.
	if err != nil {
		return "", nil, err
	}

	claims := &jwt.RegisteredClaims{}
	err = json.Unmarshal(token.Claims(), claims)
	if err != nil {
		return "", nil, err
	}

	ct, err := verify(config, ruleConfig, t)
	if err != nil {
		return "", nil, fmt.Errorf("token with algorithm %s and claims %s has error: %v", token.Header().Algorithm, string(token.Claims()), err)
	}

	return ct.UserID, token.Claims(), nil
}

func verify(verifierConf jwtverify.VerifierConfig, cfg config.Config, token string) (jwtverify.ConnectToken, error) {
	cfgContainer, err := config.NewContainer(cfg)
	if err != nil {
		return jwtverify.ConnectToken{}, err
	}
	verifier, err := jwtverify.NewTokenVerifierJWT(verifierConf, cfgContainer)
	if err != nil {
		return jwtverify.ConnectToken{}, err
	}
	return verifier.VerifyConnectToken(token, false)
}
