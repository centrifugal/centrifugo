package centrifugo

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/FZambia/viper-lite"
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/plugin"
	"github.com/centrifugal/centrifugo/libcentrifugo/server"
	"github.com/spf13/cobra"
)

func setupLogging() {
	log.SetFlags(0)
	log.SetOutput(logger.INFO)
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

func handleSignals(n *node.Node) {
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
			if err := n.Reload(viper.GetViper()); err != nil {
				logger.CRITICAL.Println(err)
				continue
			}
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
func Main(version string) {

	var configFile string

	var debug bool
	var name string
	var engn string
	var srvs string
	var admin bool
	var insecure bool
	var insecureAPI bool
	var insecureAdmin bool
	var logLevel string
	var logFile string

	var rootCmd = &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifugo. Real-time messaging (Websockets or SockJS) server in Go.",
		Run: func(cmd *cobra.Command, args []string) {

			viper.SetEnvPrefix("centrifugo")

			defaults := map[string]interface{}{
				"gomaxprocs":                     0,
				"engine":                         "memory",
				"servers":                        "http",
				"debug":                          false,
				"name":                           "",
				"secret":                         "",
				"connection_lifetime":            0,
				"watch":                          false,
				"publish":                        false,
				"anonymous":                      false,
				"presence":                       false,
				"history_size":                   0,
				"history_lifetime":               0,
				"recover":                        false,
				"history_drop_inactive":          false,
				"namespaces":                     "",
				"max_channel_length":             255,
				"user_connection_limit":          0,
				"node_ping_interval":             3,
				"ping_interval":                  25,
				"node_metrics_interval":          60,
				"stale_connection_close_delay":   25,
				"expired_connection_close_delay": 25,
				"client_message_write_timeout":   0,
				"client_channel_limit":           128,
				"client_request_max_size":        65536,    // 64KB
				"client_queue_max_size":          10485760, // 10MB
				"client_queue_initial_capacity":  2,
				"presence_ping_interval":         25,
				"presence_expire_interval":       60,
				"private_channel_prefix":         "$",
				"namespace_channel_boundary":     ":",
				"user_channel_boundary":          "#",
				"user_channel_separator":         ",",
				"client_channel_boundary":        "&",
			}

			for k, v := range defaults {
				viper.SetDefault(k, v)
			}

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

			c := node.NewConfig(viper.GetViper())
			err = c.Validate()
			if err != nil {
				logger.FATAL.Fatalln(err)
			}

			nod := node.New(version, c)

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
			logger.INFO.Printf("Centrifugo version: %s", version)
			logger.INFO.Printf("Process PID: %d", os.Getpid())
			logger.INFO.Printf("Engine: %s", e.Name())
			logger.INFO.Printf("GOMAXPROCS: %d", runtime.GOMAXPROCS(0))
			if c.Insecure {
				logger.WARN.Println("Running in INSECURE client mode")
			}
			if c.InsecureAPI {
				logger.WARN.Println("Running in INSECURE API mode")
			}
			if c.InsecureAdmin {
				logger.WARN.Println("Running in INSECURE admin mode")
			}
			if c.Debug {
				logger.WARN.Println("Running in DEBUG mode")
			}

			if err = nod.Run(&node.RunOptions{Engine: e, Servers: servers}); err != nil {
				logger.FATAL.Fatalln(err)
			}
			select {}
		},
	}

	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	rootCmd.Flags().StringVarP(&engn, "engine", "e", "memory", "engine to use: memory or redis")
	rootCmd.Flags().StringVarP(&srvs, "servers", "s", "http", "comma-separated servers to use")
	rootCmd.Flags().StringVarP(&name, "name", "n", "", "unique node name")
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "enable debug mode")
	rootCmd.Flags().BoolVarP(&admin, "admin", "", false, "enable admin socket")
	rootCmd.Flags().BoolVarP(&insecure, "insecure", "", false, "start in insecure client mode")
	rootCmd.Flags().BoolVarP(&insecureAPI, "insecure_api", "", false, "use insecure API mode")
	rootCmd.Flags().BoolVarP(&insecureAdmin, "insecure_admin", "", false, "use insecure admin mode – no auth required for admin socket")

	rootCmd.Flags().StringVarP(&logLevel, "log_level", "", "info", "set the log level: trace, debug, info, error, critical, fatal or none")
	rootCmd.Flags().StringVarP(&logFile, "log_file", "", "", "optional log file - if not specified logs go to STDOUT")

	for _, configurator := range plugin.Configurators {
		configurator(config.NewViperConfigSetter(viper.GetViper(), rootCmd.Flags()))
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Centrifugo version number",
		Long:  `Print the version number of Centrifugo`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Centrifugo v%s (Go version: %s)\n", version, runtime.Version())
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
