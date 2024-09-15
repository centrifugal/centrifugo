// Copyright (c) 2015 Centrifugal
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.
package main

import (
	"github.com/centrifugal/centrifugo/v5/internal/cli"
	"github.com/centrifugal/centrifugo/v5/internal/config"
	"github.com/centrifugal/centrifugo/v5/internal/runutil"

	"github.com/spf13/cobra"
)

//go:generate go run internal/gen/api/main.go

func main() {
	var configFile string

	var rootCmd = &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifugo – scalable real-time messaging server in language-agnostic way",
		Run: func(cmd *cobra.Command, args []string) {
			runutil.Run(cmd, configFile)
		},
	}
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	config.DefineFlags(rootCmd)

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Centrifugo version information",
		Long:  `Print the version information of Centrifugo`,
		Run: func(cmd *cobra.Command, args []string) {
			cli.Version()
		},
	}

	var checkConfigFile string
	var checkConfigCmd = &cobra.Command{
		Use:   "checkconfig",
		Short: "Check configuration file",
		Long:  `Check Centrifugo configuration file`,
		Run: func(cmd *cobra.Command, args []string) {
			cli.CheckConfig(cmd, checkConfigFile)
		},
	}
	checkConfigCmd.Flags().StringVarP(&checkConfigFile, "config", "c", "config.json", "path to config file to check")

	var outputConfigFile string
	var genConfigCmd = &cobra.Command{
		Use:   "genconfig",
		Short: "Generate minimal configuration file to start with",
		Long:  `Generate minimal configuration file to start with`,
		Run: func(cmd *cobra.Command, args []string) {
			cli.GenConfig(cmd, outputConfigFile)
		},
	}
	genConfigCmd.Flags().StringVarP(&outputConfigFile, "config", "c", "config.json", "path to output config file")

	var genTokenConfigFile string
	var genTokenUser string
	var genTokenTTL int64
	var genTokenQuiet bool
	var genTokenCmd = &cobra.Command{
		Use:   "gentoken",
		Short: "Generate sample connection JWT for user",
		Long:  `Generate sample connection JWT for user`,
		Run: func(cmd *cobra.Command, args []string) {
			cli.GenToken(cmd, genTokenConfigFile, genTokenUser, genTokenTTL, genTokenQuiet)
		},
	}
	genTokenCmd.Flags().StringVarP(&genTokenConfigFile, "config", "c", "config.json", "path to config file")
	genTokenCmd.Flags().StringVarP(&genTokenUser, "user", "u", "", "user ID, by default anonymous")
	genTokenCmd.Flags().Int64VarP(&genTokenTTL, "ttl", "t", 3600*24*7, "token TTL in seconds, use -1 for token without expiration")
	genTokenCmd.Flags().BoolVarP(&genTokenQuiet, "quiet", "q", false, "only output the token without anything else")

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
			cli.GenSubToken(cmd, genSubTokenConfigFile, genSubTokenUser, genSubTokenChannel, genSubTokenTTL, genSubTokenQuiet)
		},
	}
	genSubTokenCmd.Flags().StringVarP(&genSubTokenConfigFile, "config", "c", "config.json", "path to config file")
	genSubTokenCmd.Flags().StringVarP(&genSubTokenUser, "user", "u", "", "user ID")
	genSubTokenCmd.Flags().StringVarP(&genSubTokenChannel, "channel", "s", "", "channel")
	genSubTokenCmd.Flags().Int64VarP(&genSubTokenTTL, "ttl", "t", 3600*24*7, "token TTL in seconds, use -1 for token without expiration")
	genSubTokenCmd.Flags().BoolVarP(&genSubTokenQuiet, "quiet", "q", false, "only output the token without anything else")

	var checkTokenConfigFile string
	var checkTokenCmd = &cobra.Command{
		Use:   "checktoken [TOKEN]",
		Short: "Check connection JWT",
		Long:  `Check connection JWT`,
		Run: func(cmd *cobra.Command, args []string) {
			cli.CheckToken(cmd, checkTokenConfigFile, args)
		},
	}
	checkTokenCmd.Flags().StringVarP(&checkTokenConfigFile, "config", "c", "config.json", "path to config file")

	var checkSubTokenConfigFile string
	var checkSubTokenCmd = &cobra.Command{
		Use:   "checksubtoken [TOKEN]",
		Short: "Check subscription JWT",
		Long:  `Check subscription JWT`,
		Run: func(cmd *cobra.Command, args []string) {
			cli.CheckSubToken(cmd, checkSubTokenConfigFile, args)
		},
	}
	checkSubTokenCmd.Flags().StringVarP(&checkSubTokenConfigFile, "config", "c", "config.json", "path to config file")

	var serveDir string
	var servePort int
	var serveAddr string
	var serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "Run static file server (for development only)",
		Long:  `Run static file server (for development only)`,
		Run: func(cmd *cobra.Command, args []string) {
			cli.Serve(serveAddr, servePort, serveDir)
		},
	}
	serveCmd.Flags().StringVarP(&serveDir, "dir", "d", "./", "path to directory")
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 3000, "port to serve on")
	serveCmd.Flags().StringVarP(&serveAddr, "address", "a", "", "interface to serve on (default: all interfaces)")

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(checkConfigCmd)
	rootCmd.AddCommand(genConfigCmd)
	rootCmd.AddCommand(genTokenCmd)
	rootCmd.AddCommand(genSubTokenCmd)
	rootCmd.AddCommand(checkTokenCmd)
	rootCmd.AddCommand(checkSubTokenCmd)
	_ = rootCmd.Execute()
}
