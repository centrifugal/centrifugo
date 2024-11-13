// Copyright (c) Centrifugal Labs LTD
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
	"github.com/centrifugal/centrifugo/v5/internal/app"
	"github.com/centrifugal/centrifugo/v5/internal/cli"
	"github.com/centrifugal/centrifugo/v5/internal/config"

	"github.com/spf13/cobra"
)

//go:generate go run internal/gen/api/main.go

func main() {
	var configFile string
	rootCmd := &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifugo – scalable real-time messaging server in language-agnostic way",
		Run: func(cmd *cobra.Command, args []string) {
			app.Run(cmd, configFile)
		},
	}
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	config.DefineFlags(rootCmd)

	// Add additional helper CLI, see `centrifugo -h` and https://centrifugal.dev/docs/server/console_commands.
	rootCmd.AddCommand(cli.ServeCommand())
	rootCmd.AddCommand(cli.VersionCommand())
	rootCmd.AddCommand(cli.CheckConfigCommand())
	rootCmd.AddCommand(cli.GenConfigCommand())
	rootCmd.AddCommand(cli.GenTokenCommand())
	rootCmd.AddCommand(cli.GenSubTokenCommand())
	rootCmd.AddCommand(cli.CheckTokenCommand())
	rootCmd.AddCommand(cli.CheckSubTokenCommand())
	rootCmd.AddCommand(cli.DefaultConfigCommand())
	rootCmd.AddCommand(cli.DefaultEnvCommand())

	_ = rootCmd.Execute()
}
