package main

import (
	"net/http"
	//"net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/centrifugal/centrifugo/logger"

	"github.com/julienschmidt/httprouter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	VERSION = "0.8.0"
)

func setupLogging() {
	logLevel, ok := logger.LevelMatches[strings.ToUpper(viper.GetString("logging"))]
	if !ok {
		logLevel = logger.LevelInfo
	}
	logger.SetLogThreshold(logLevel)
	logger.SetStdoutThreshold(logLevel)

	if viper.IsSet("logfile") && viper.GetString("logfile") != "" {
		logger.SetLogFile(viper.GetString("logfile"))
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
	var configFile string
	var logging string
	var logFile string

	var rootCmd = &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifuge in GO",
		Run: func(cmd *cobra.Command, args []string) {

			viper.SetDefault("password", "")
			viper.SetDefault("cookie_secret", "cookie_secret")
			viper.SetDefault("api_secret", "api_secret")
			viper.SetDefault("max_channel_length", 255)
			viper.SetDefault("channel_prefix", "centrifugo")
			viper.SetDefault("presence_ping_interval", 25)
			viper.SetDefault("presence_expire_interval", 60)
			viper.SetDefault("private_channel_prefix", "$")
			viper.SetDefault("namespace_channel_boundary", ":")
			viper.SetDefault("user_channel_boundary", "#")
			viper.SetDefault("user_channel_separator", ",")

			viper.SetConfigFile(configFile)

			viper.SetEnvPrefix("centrifuge")
			viper.BindEnv("structure", "engine", "insecure", "password", "cookie_secret", "api_secret")

			viper.BindPFlag("port", cmd.Flags().Lookup("port"))
			viper.BindPFlag("address", cmd.Flags().Lookup("address"))
			viper.BindPFlag("debug", cmd.Flags().Lookup("debug"))
			viper.BindPFlag("name", cmd.Flags().Lookup("name"))
			viper.BindPFlag("web", cmd.Flags().Lookup("web"))
			viper.BindPFlag("logging", cmd.Flags().Lookup("logging"))
			viper.BindPFlag("logfile", cmd.Flags().Lookup("logfile"))

			err := viper.ReadInConfig()
			if err != nil {
				panic("unable to locate config file")
			}
			setupLogging()

			logger.INFO.Println("using config file:", viper.ConfigFileUsed())
			logger.DEBUG.Printf("%v\n", viper.AllSettings())

			app, err := newApplication()
			if err != nil {
				panic(err)
			}

			e := newMemoryEngine(app)
			app.setEngine(e)

			app.initialize()

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
			router.GET("/info/", app.infoHandler)
			router.POST("/actions/", app.actionsHandler)

			//if viper.GetBool("debug") {
			//	router.HandlerFunc("GET", "/debug/pprof/*path", http.HandlerFunc(pprof.Index))
			//}

			tick := time.Tick(10 * time.Second)
			go func() {
				for {
					select {
					case <-tick:
						for ch, val := range app.clientSubscriptionHub.subscriptions {
							logger.INFO.Printf("%s: %d\n", ch, len(val))
						}
					}
				}
			}()

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
	rootCmd.Flags().StringVarP(&logging, "logging", "l", "info", "set the log level: debug, info, error, critical, fatal or none")
	rootCmd.Flags().StringVarP(&logFile, "logfile", "f", "", "optional log file - if not specified all logs go to STDOUT")
	rootCmd.Execute()
}
