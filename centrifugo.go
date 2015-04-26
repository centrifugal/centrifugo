package main

import (
	"net/http"
	//"net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/centrifugal/centrifugo/logger"

	"github.com/julienschmidt/httprouter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	VERSION = "0.8.0"
)

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
			logger.INFO.Println("reload configuration")
			viper.ReadInConfig()
			app.initialize()
			setupLogging()
		}
	}
}

func main() {

	var port string
	var address string
	var debug bool
	var name string
	var web string
	var engn string
	var configFile string
	var logLevel string
	var logFile string

	var redisHost string
	var redisPort string
	var redisPassword string
	var redisDb string
	var redisUrl string
	var redisApi bool

	var rootCmd = &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifuge in GO",
		Run: func(cmd *cobra.Command, args []string) {

			viper.SetDefault("password", "")
			viper.SetDefault("secret", "secret")
			viper.RegisterAlias("cookie_secret", "secret")
			viper.SetDefault("max_channel_length", 255)
			viper.SetDefault("channel_prefix", "centrifugo")
			viper.SetDefault("node_ping_interval", 5)
			viper.SetDefault("expired_connection_close_delay", 10)
			viper.SetDefault("presence_ping_interval", 25)
			viper.SetDefault("presence_expire_interval", 60)
			viper.SetDefault("private_channel_prefix", "$")
			viper.SetDefault("namespace_channel_boundary", ":")
			viper.SetDefault("user_channel_boundary", "#")
			viper.SetDefault("user_channel_separator", ",")

			viper.SetConfigFile(configFile)

			viper.SetEnvPrefix("centrifuge")
			viper.BindEnv("structure", "engine", "insecure", "password", "cookie_secret")

			viper.BindPFlag("port", cmd.Flags().Lookup("port"))
			viper.BindPFlag("address", cmd.Flags().Lookup("address"))
			viper.BindPFlag("debug", cmd.Flags().Lookup("debug"))
			viper.BindPFlag("name", cmd.Flags().Lookup("name"))
			viper.BindPFlag("web", cmd.Flags().Lookup("web"))
			viper.BindPFlag("engine", cmd.Flags().Lookup("engine"))
			viper.BindPFlag("log_level", cmd.Flags().Lookup("log_level"))
			viper.BindPFlag("log_file", cmd.Flags().Lookup("log_file"))
			viper.BindPFlag("redis_host", cmd.Flags().Lookup("redis_host"))
			viper.BindPFlag("redis_port", cmd.Flags().Lookup("redis_port"))
			viper.BindPFlag("redis_password", cmd.Flags().Lookup("redis_password"))
			viper.BindPFlag("redis_db", cmd.Flags().Lookup("redis_db"))
			viper.BindPFlag("redis_url", cmd.Flags().Lookup("redis_url"))
			viper.BindPFlag("redis_api", cmd.Flags().Lookup("redis_api"))

			err := viper.ReadInConfig()
			if err != nil {
				panic("unable to locate config file")
			}
			setupLogging()

			logger.INFO.Println("using config file:", viper.ConfigFileUsed())

			app, err := newApplication()
			if err != nil {
				panic(err)
			}

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
				)
			default:
				panic("unknown engine: " + viper.GetString("engine"))
			}
			app.setEngine(e)

			logger.INFO.Println("engine:", viper.GetString("engine"))
			logger.DEBUG.Printf("%v\n", viper.AllSettings())

			app.initialize()
			app.run()

			go handleSignals(app)

			router := httprouter.New()

			// register SockJS endpoints
			sockJSHandler := newClientConnectionHandler(app)
			router.Handler("GET", "/connection/*path", sockJSHandler)
			router.Handler("POST", "/connection/*path", sockJSHandler)
			router.Handler("OPTIONS", "/connection/*path", sockJSHandler)

			// register HTTP API endpoint
			router.POST("/api/:projectKey", app.apiHandler)

			// register admin web interface API endpoints
			router.POST("/auth/", app.authHandler)
			router.GET("/info/", app.Authenticated(app.infoHandler))
			router.POST("/action/", app.Authenticated(app.actionHandler))
			router.Handler("GET", "/socket", wsHandler{app: app})
			/*
				if viper.GetBool("debug") {
					router.HandlerFunc("GET", "/debug/pprof/*path", http.HandlerFunc(pprof.Index))
				}
			*/

			// optionally serve admin web interface application
			webDir := viper.GetString("web")
			if webDir != "" {
				router.Handler("GET", "/", http.FileServer(http.Dir(webDir)))
				router.Handler("GET", "/public/*filepath", http.FileServer(http.Dir(webDir)))
			}

			addr := viper.GetString("address") + ":" + viper.GetString("port")
			if err := http.ListenAndServe(addr, router); err != nil {
				logger.FATAL.Fatalln("ListenAndServe:", err)
			}
		},
	}
	rootCmd.Flags().StringVarP(&port, "port", "p", "8000", "port")
	rootCmd.Flags().StringVarP(&address, "address", "a", "localhost", "address")
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "debug mode - please, do not use it in production")
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	rootCmd.Flags().StringVarP(&name, "name", "n", "", "unique node name")
	rootCmd.Flags().StringVarP(&web, "web", "w", "", "optional path to web interface application")
	rootCmd.Flags().StringVarP(&engn, "engine", "e", "memory", "engine to use: memory or redis")
	rootCmd.Flags().StringVarP(&logLevel, "log_level", "", "info", "set the log level: debug, info, error, critical, fatal or none")
	rootCmd.Flags().StringVarP(&logFile, "log_file", "", "", "optional log file - if not specified all logs go to STDOUT")
	rootCmd.Flags().StringVarP(&redisHost, "redis_host", "", "127.0.0.1", "redis host (Redis engine)")
	rootCmd.Flags().StringVarP(&redisPort, "redis_port", "", "6379", "redis port (Redis engine)")
	rootCmd.Flags().StringVarP(&redisPassword, "redis_password", "", "", "redis auth password (Redis engine)")
	rootCmd.Flags().StringVarP(&redisDb, "redis_db", "", "0", "redis database (Redis engine)")
	rootCmd.Flags().StringVarP(&redisUrl, "redis_url", "", "", "redis connection URL (Redis engine)")
	rootCmd.Flags().BoolVarP(&redisApi, "redis_api", "", false, "enable Redis API listener (Redis engine)")
	rootCmd.Execute()
}
