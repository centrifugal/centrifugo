package libcentrifugo

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/klauspost/shutdown"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	VERSION = "0.1.1"
)

var configFile string

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

func handleSignals(app *application) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP)
	for {
		sig := <-sigc
		logger.INFO.Println("signal received:", sig)
		switch sig {
		case syscall.SIGHUP:
			// reload application configuration on SIGHUP
			// note that you should run checkconfig before reloading configuration
			// as Viper exits when encounters parsing errors
			logger.INFO.Println("reloading configuration")
			err := viper.ReadInConfig()
			if err != nil {
				logger.CRITICAL.Println("unable to locate config file")
				return
			}
			setupLogging()
			app.initialize()
		}
	}
}

func Main() {

	var port string
	var address string
	var debug bool
	var name string
	var web string
	var engn string
	var logLevel string
	var logFile string
	var insecure bool

	var redisHost string
	var redisPort string
	var redisPassword string
	var redisDB string
	var redisURL string
	var redisAPI bool
	var redisPool int

	var rootCmd = &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifuge + GO = Centrifugo – harder, better, faster, stronger",
		Run: func(cmd *cobra.Command, args []string) {

			viper.SetDefault("gomaxprocs", 0)
			viper.SetDefault("web_password", "")
			viper.SetDefault("web_secret", "")
			viper.RegisterAlias("cookie_secret", "")
			viper.SetDefault("max_channel_length", 255)
			viper.SetDefault("channel_prefix", "centrifugo")
			viper.SetDefault("node_ping_interval", 5)
			viper.SetDefault("message_send_timeout", 60)
			viper.SetDefault("expired_connection_close_delay", 10)
			viper.SetDefault("presence_ping_interval", 25)
			viper.SetDefault("presence_expire_interval", 60)
			viper.SetDefault("private_channel_prefix", "$")
			viper.SetDefault("namespace_channel_boundary", ":")
			viper.SetDefault("user_channel_boundary", "#")
			viper.SetDefault("user_channel_separator", ",")
			viper.SetDefault("sockjs_url", "https://cdn.jsdelivr.net/sockjs/1.0/sockjs.min.js")

			viper.SetDefault("project_name", "")
			viper.SetDefault("project_secret", "")
			viper.SetDefault("project_connection_lifetime", false)
			viper.SetDefault("project_watch", false)
			viper.SetDefault("project_publish", false)
			viper.SetDefault("project_anonymous", false)
			viper.SetDefault("project_presence", false)
			viper.SetDefault("project_history_size", 0)
			viper.SetDefault("project_history_lifetime", 0)
			viper.SetDefault("project_namespaces", "")

			viper.SetEnvPrefix("centrifugo")
			viper.BindEnv("engine")
			viper.BindEnv("insecure")
			viper.BindEnv("web_password")
			viper.BindEnv("web_secret")
			viper.BindEnv("project_name")
			viper.BindEnv("project_secret")
			viper.BindEnv("project_connection_lifetime")
			viper.BindEnv("project_watch")
			viper.BindEnv("project_publish")
			viper.BindEnv("project_anonymous")
			viper.BindEnv("project_join_leave")
			viper.BindEnv("project_presence")
			viper.BindEnv("project_history_size")
			viper.BindEnv("project_history_lifetime")

			viper.BindPFlag("port", cmd.Flags().Lookup("port"))
			viper.BindPFlag("address", cmd.Flags().Lookup("address"))
			viper.BindPFlag("debug", cmd.Flags().Lookup("debug"))
			viper.BindPFlag("name", cmd.Flags().Lookup("name"))
			viper.BindPFlag("web", cmd.Flags().Lookup("web"))
			viper.BindPFlag("engine", cmd.Flags().Lookup("engine"))
			viper.BindPFlag("insecure", cmd.Flags().Lookup("insecure"))
			viper.BindPFlag("log_level", cmd.Flags().Lookup("log_level"))
			viper.BindPFlag("log_file", cmd.Flags().Lookup("log_file"))
			viper.BindPFlag("redis_host", cmd.Flags().Lookup("redis_host"))
			viper.BindPFlag("redis_port", cmd.Flags().Lookup("redis_port"))
			viper.BindPFlag("redis_password", cmd.Flags().Lookup("redis_password"))
			viper.BindPFlag("redis_db", cmd.Flags().Lookup("redis_db"))
			viper.BindPFlag("redis_url", cmd.Flags().Lookup("redis_url"))
			viper.BindPFlag("redis_api", cmd.Flags().Lookup("redis_api"))
			viper.BindPFlag("redis_pool", cmd.Flags().Lookup("redis_pool"))

			err := validateConfig(configFile)
			if err != nil {
				logger.FATAL.Fatalln(err)
			}

			viper.SetConfigFile(configFile)
			err = viper.ReadInConfig()
			if err != nil {
				logger.FATAL.Fatalln("unable to locate config file")
			}
			setupLogging()

			if os.Getenv("GOMAXPROCS") == "" {
				if viper.IsSet("gomaxprocs") && viper.GetInt("gomaxprocs") > 0 {
					runtime.GOMAXPROCS(viper.GetInt("gomaxprocs"))
				} else {
					runtime.GOMAXPROCS(runtime.NumCPU())
				}
			}

			logger.INFO.Println("GOMAXPROCS set to", runtime.GOMAXPROCS(0))
			logger.INFO.Println("using config file:", viper.ConfigFileUsed())

			app, err := newApplication()
			if err != nil {
				logger.FATAL.Fatalln(err)
			}
			app.initialize()

			var e engine
			switch viper.GetString("engine") {
			case "memory":
				e = newMemoryEngine(app)
			case "redis":
				e = newRedisEngine(
					app,
					viper.GetString("redis_host"),
					viper.GetString("redis_port"),
					viper.GetString("redis_password"),
					viper.GetString("redis_db"),
					viper.GetString("redis_url"),
					viper.GetBool("redis_api"),
					viper.GetInt("redis_pool"),
				)
			default:
				logger.FATAL.Fatalln("unknown engine: " + viper.GetString("engine"))
			}

			logger.INFO.Println("engine:", viper.GetString("engine"))
			logger.DEBUG.Printf("%v\n", viper.AllSettings())

			app.setEngine(e)

			err = e.initialize()
			if err != nil {
				logger.FATAL.Fatalln(err)
			}

			app.run()

			shutdown.OnSignal(0, os.Interrupt, syscall.SIGTERM)
			go handleSignals(app)

			// register raw Websocket endpoint
			http.Handle("/connection/websocket", app.Logged(http.HandlerFunc(app.rawWebsocketHandler)))

			// register SockJS endpoints
			http.Handle("/connection/", app.Logged(newSockJSHandler(app, viper.GetString("sockjs_url"))))

			// register HTTP API endpoint
			http.Handle("/api/", app.Logged(shutdown.WrapHandler(http.HandlerFunc(app.apiHandler))))

			// register admin web interface API endpoints
			http.Handle("/auth/", app.Logged(shutdown.WrapHandler(http.HandlerFunc(app.authHandler))))
			http.Handle("/info/", app.Logged(shutdown.WrapHandler(app.Authenticated(http.HandlerFunc(app.infoHandler)))))
			http.Handle("/action/", app.Logged(shutdown.WrapHandler(app.Authenticated(http.HandlerFunc(app.actionHandler)))))
			http.Handle("/socket", app.Logged(shutdown.WrapHandler(http.HandlerFunc(app.adminWebsocketHandler))))

			// optionally serve admin web interface application
			webDir := viper.GetString("web")
			if webDir != "" {
				http.Handle("/", shutdown.WrapHandler(http.FileServer(http.Dir(webDir))))
			}

			addr := viper.GetString("address") + ":" + viper.GetString("port")
			logger.INFO.Printf("start serving on %s\n", addr)
			if err := http.ListenAndServe(addr, nil); err != nil {
				logger.FATAL.Fatalln("ListenAndServe:", err)
			}
		},
	}
	rootCmd.Flags().StringVarP(&port, "port", "p", "8000", "port to bind to")
	rootCmd.Flags().StringVarP(&address, "address", "a", "", "address to listen on")
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "debug mode - please, do not use it in production")
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	rootCmd.Flags().StringVarP(&name, "name", "n", "", "unique node name")
	rootCmd.Flags().StringVarP(&web, "web", "w", "", "optional path to web interface application")
	rootCmd.Flags().StringVarP(&engn, "engine", "e", "memory", "engine to use: memory or redis")
	rootCmd.Flags().BoolVarP(&insecure, "insecure", "", false, "start in insecure mode")
	rootCmd.Flags().StringVarP(&logLevel, "log_level", "", "info", "set the log level: debug, info, error, critical, fatal or none")
	rootCmd.Flags().StringVarP(&logFile, "log_file", "", "", "optional log file - if not specified all logs go to STDOUT")
	rootCmd.Flags().StringVarP(&redisHost, "redis_host", "", "127.0.0.1", "redis host (Redis engine)")
	rootCmd.Flags().StringVarP(&redisPort, "redis_port", "", "6379", "redis port (Redis engine)")
	rootCmd.Flags().StringVarP(&redisPassword, "redis_password", "", "", "redis auth password (Redis engine)")
	rootCmd.Flags().StringVarP(&redisDB, "redis_db", "", "0", "redis database (Redis engine)")
	rootCmd.Flags().StringVarP(&redisURL, "redis_url", "", "", "redis connection URL (Redis engine)")
	rootCmd.Flags().BoolVarP(&redisAPI, "redis_api", "", false, "enable Redis API listener (Redis engine)")
	rootCmd.Flags().IntVarP(&redisPool, "redis_pool", "", 256, "Redis pool size (Redis engine)")

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Centrifugo version number",
		Long:  `Print the version number of Centrifugo`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Centrifugo v%s\n", VERSION)
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
