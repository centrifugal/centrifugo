package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/FZambia/go-logger"
	"github.com/FZambia/viper-lite"
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/plugin"
	"github.com/centrifugal/centrifugo/libcentrifugo/server"
	"github.com/spf13/cobra"

	// Register builtin memory and redis engines.
	_ "github.com/centrifugal/centrifugo/libcentrifugo/engine/enginememory"
	_ "github.com/centrifugal/centrifugo/libcentrifugo/engine/engineredis"

	// Register servers.
	_ "github.com/centrifugal/centrifugo/libcentrifugo/server/httpserver"

	// Register embedded web interface.
	_ "github.com/centrifugal/centrifugo/libcentrifugo/statik"
)

// Version of Centrifugo server. Set on build stage.
var VERSION string

func setupLogging() {
	logLevel, ok := logger.LevelMatches[strings.ToUpper(viper.GetString("log_level"))]
	if !ok {
		logLevel = logger.LevelInfo
	}
	logger.SetLogThreshold(logLevel)
	logger.SetStdoutThreshold(logLevel)

	if viper.IsSet("log_file") && viper.GetString("log_file") != "" {
		logger.SetLogFile(viper.GetString("log_file"))
		// do not log into stdout when log file provided
		logger.SetStdoutThreshold(logger.LevelNone)
	}
}

func handleSignals(n node.Node) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP, syscall.SIGINT, os.Interrupt, syscall.SIGTERM)
	for {
		sig := <-sigc
		logger.INFO.Println("Signal received:", sig)
		switch sig {
		case syscall.SIGHUP:
			// reload application configuration on SIGHUP
			logger.INFO.Println("Reloading configuration")
			err := viper.ReadInConfig()
			if err != nil {
				switch err.(type) {
				case viper.ConfigParseError:
					logger.CRITICAL.Printf("Error parsing configuration: %s\n", err)
					continue
				default:
					logger.CRITICAL.Println("No config file found")
					continue
				}
			}
			setupLogging()
			c := newConfig(viper.GetViper())
			if err := c.Validate(); err != nil {
				logger.CRITICAL.Println(err)
				continue
			}
			n.SetConfig(c)
			logger.INFO.Println("Configuration successfully reloaded")
		case syscall.SIGINT, os.Interrupt, syscall.SIGTERM:
			logger.INFO.Println("Shutting down")
			go time.AfterFunc(10*time.Second, func() {
				os.Exit(1)
			})
			n.Shutdown()
			os.Exit(0)
		}
	}
}

// Main starts Centrifugo server.
func Main() {

	var configFile string

	var debug bool
	var name string
	var admin bool
	var insecureAdmin bool
	var engn string
	var srvs string
	var logLevel string
	var logFile string
	var insecure bool
	var insecureAPI bool

	var rootCmd = &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifugo. Real-time messaging (Websockets or SockJS) server in Go.",
		Run: func(cmd *cobra.Command, args []string) {

			viper.SetEnvPrefix("centrifugo")

			viper.SetDefault("gomaxprocs", 0)
			viper.SetDefault("engine", "memory")
			viper.SetDefault("servers", "http")

			viper.SetDefault("debug", false)
			viper.SetDefault("name", "")
			viper.SetDefault("max_channel_length", 255)
			viper.SetDefault("node_ping_interval", 3)
			viper.SetDefault("message_send_timeout", 0)
			viper.SetDefault("ping_interval", 25)
			viper.SetDefault("node_metrics_interval", 60)
			viper.SetDefault("stale_connection_close_delay", 25)
			viper.SetDefault("expired_connection_close_delay", 25)
			viper.SetDefault("client_channel_limit", 100)
			viper.SetDefault("client_request_max_size", 65536)  // 64KB
			viper.SetDefault("client_queue_max_size", 10485760) // 10MB
			viper.SetDefault("client_queue_initial_capacity", 2)
			viper.SetDefault("presence_ping_interval", 25)
			viper.SetDefault("presence_expire_interval", 60)
			viper.SetDefault("private_channel_prefix", "$")
			viper.SetDefault("namespace_channel_boundary", ":")
			viper.SetDefault("user_channel_boundary", "#")
			viper.SetDefault("user_channel_separator", ",")
			viper.SetDefault("client_channel_boundary", "&")
			viper.SetDefault("secret", "")
			viper.SetDefault("connection_lifetime", 0)
			viper.SetDefault("watch", false)
			viper.SetDefault("publish", false)
			viper.SetDefault("anonymous", false)
			viper.SetDefault("presence", false)
			viper.SetDefault("history_size", 0)
			viper.SetDefault("history_lifetime", 0)
			viper.SetDefault("recover", false)
			viper.SetDefault("history_drop_inactive", false)
			viper.SetDefault("namespaces", "")

			bindEnvs := []string{
				"debug", "engine", "insecure", "insecure_api", "admin", "admin_password", "admin_secret",
				"insecure_admin", "secret", "connection_lifetime", "watch", "publish", "anonymous", "join_leave",
				"presence", "recover", "history_size", "history_lifetime", "history_drop_inactive",
			}
			for _, env := range bindEnvs {
				viper.BindEnv(env)
			}

			bindPFlags := []string{
				"debug", "name", "admin", "insecure_admin", "engine", "servers", "insecure",
				"insecure_api", "log_level", "log_file",
			}
			for _, flag := range bindPFlags {
				viper.BindPFlag(flag, cmd.Flags().Lookup(flag))
			}

			viper.SetConfigFile(configFile)

			absConfPath, err := filepath.Abs(configFile)
			if err != nil {
				logger.FATAL.Fatalln(err)
			}

			err = viper.ReadInConfig()
			configFound := true
			if err != nil {
				switch err.(type) {
				case viper.ConfigParseError:
					logger.FATAL.Fatalf("Error parsing configuration: %s\n", err)
				default:
					configFound = false
				}
			}

			setupLogging()

			if !configFound {
				logger.WARN.Println("No config file found")
			}

			if os.Getenv("GOMAXPROCS") == "" {
				if viper.IsSet("gomaxprocs") && viper.GetInt("gomaxprocs") > 0 {
					runtime.GOMAXPROCS(viper.GetInt("gomaxprocs"))
				} else {
					runtime.GOMAXPROCS(runtime.NumCPU())
				}
			}

			c := newConfig(viper.GetViper())
			err = c.Validate()
			if err != nil {
				logger.FATAL.Fatalln(err)
			}

			nod := node.New(c)

			engineName := viper.GetString("engine")
			engineFactory, ok := plugin.EngineFactories[engineName]
			if !ok {
				logger.FATAL.Fatalln("Unknown engine: " + engineName)
			}
			var e engine.Engine
			e, err = engineFactory(nod, viper.GetViper())
			if err != nil {
				logger.FATAL.Fatalln(err)
			}

			serverNames := strings.Split(viper.GetString("servers"), ",")
			servers := map[string]server.Server{}
			for _, serverName := range serverNames {
				fn, ok := plugin.ServerFactories[serverName]
				if !ok {
					logger.FATAL.Printf("Server %s not registered", serverName)
					continue
				}
				srv, err := fn(nod, viper.GetViper())
				if err != nil {
					logger.FATAL.Fatalln(err)
				}
				servers[serverName] = srv
			}

			go handleSignals(nod)

			logger.INFO.Printf("Config path: %s", absConfPath)
			logger.INFO.Printf("Centrifugo version: %s", VERSION)
			logger.INFO.Printf("Process PID: %d", os.Getpid())
			logger.INFO.Printf("Engine: %s", e.Name())
			logger.INFO.Printf("GOMAXPROCS: %d", runtime.GOMAXPROCS(0))

			if err = nod.Run(&node.NodeRunOptions{Engine: e, Servers: servers}); err != nil {
				logger.FATAL.Fatalln(err)
			}
		},
	}

	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	rootCmd.Flags().StringVarP(&engn, "engine", "e", "memory", "engine to use: memory or redis")
	rootCmd.Flags().StringVarP(&srvs, "servers", "s", "http", "comma-separated servers to use")
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "enable debug mode")
	rootCmd.Flags().StringVarP(&name, "name", "n", "", "unique node name")
	rootCmd.Flags().BoolVarP(&admin, "admin", "", false, "enable admin socket")
	rootCmd.Flags().BoolVarP(&insecure, "insecure", "", false, "start in insecure client mode")
	rootCmd.Flags().BoolVarP(&insecureAPI, "insecure_api", "", false, "use insecure API mode")
	rootCmd.Flags().BoolVarP(&insecureAdmin, "insecure_admin", "", false, "use insecure admin mode – no auth required for admin socket")

	rootCmd.Flags().StringVarP(&logLevel, "log_level", "", "debug", "set the log level: trace, debug, info, error, critical, fatal or none")
	rootCmd.Flags().StringVarP(&logFile, "log_file", "", "", "optional log file - if not specified logs go to STDOUT")

	for _, configurator := range plugin.Configurators {
		configurator(plugin.NewViperConfigSetter(viper.GetViper(), rootCmd.Flags()))
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Centrifugo version number",
		Long:  `Print the version number of Centrifugo`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Centrifugo v%s (Go version: %s)\n", VERSION, runtime.Version())
		},
	}

	var checkConfigFile string

	var checkConfigCmd = &cobra.Command{
		Use:   "checkconfig",
		Short: "Check configuration file",
		Long:  `Check Centrifugo configuration file`,
		Run: func(cmd *cobra.Command, args []string) {
			err := validateConfig(checkConfigFile)
			if err != nil {
				logger.FATAL.Fatalln(err)
			}
		},
	}
	checkConfigCmd.Flags().StringVarP(&checkConfigFile, "config", "c", "config.json", "path to config file to check")

	var outputConfigFile string

	var generateConfigCmd = &cobra.Command{
		Use:   "genconfig",
		Short: "Generate simple configuration file to start with",
		Long:  `Generate simple configuration file to start with`,
		Run: func(cmd *cobra.Command, args []string) {
			err := generateConfig(outputConfigFile)
			if err != nil {
				logger.FATAL.Fatalln(err)
			}
		},
	}
	generateConfigCmd.Flags().StringVarP(&outputConfigFile, "config", "c", "config.json", "path to output config file")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(checkConfigCmd)
	rootCmd.AddCommand(generateConfigCmd)
	rootCmd.Execute()
}

func main() {
	Main()
}
