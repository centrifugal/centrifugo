package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/libcentrifugo"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/igm/sockjs-go.v2/sockjs"
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

func handleSignals(app *libcentrifugo.Application) {
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
			c := newConfig()
			app.SetConfig(c)
			logger.INFO.Println("Configuration successfully reloaded")
		case syscall.SIGINT, os.Interrupt, syscall.SIGTERM:
			logger.INFO.Println("Shutting down")
			go time.AfterFunc(10*time.Second, func() {
				os.Exit(1)
			})
			app.Shutdown()
			os.Exit(0)
		}
	}
}

func listenHTTP(mux http.Handler, addr string, useSSL bool, sslCert, sslKey string, wg *sync.WaitGroup) {
	defer wg.Done()
	if useSSL {
		if err := http.ListenAndServeTLS(addr, sslCert, sslKey, mux); err != nil {
			logger.FATAL.Fatalln("ListenAndServe:", err)
		}
	} else {
		if err := http.ListenAndServe(addr, mux); err != nil {
			logger.FATAL.Fatalln("ListenAndServe:", err)
		}
	}
}

// Main starts Centrifugo server.
func Main() {

	var configFile string

	var port string
	var address string
	var debug bool
	var name string
	var admin bool
	var insecureAdmin bool
	var web bool
	var webPath string
	var insecureWeb bool
	var engn string
	var logLevel string
	var logFile string
	var insecure bool
	var insecureAPI bool
	var useSSL bool
	var sslCert string
	var sslKey string
	var apiPort string
	var adminPort string

	var redisHost string
	var redisPort string
	var redisPassword string
	var redisDB string
	var redisURL string
	var redisAPI bool
	var redisPool int
	var redisAPINumShards int
	var redisMasterName string
	var redisSentinels string

	var rootCmd = &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifugo. Real-time messaging (Websockets or SockJS) server in Go.",
		Run: func(cmd *cobra.Command, args []string) {

			viper.SetDefault("gomaxprocs", 0)
			viper.SetDefault("debug", false)
			viper.SetDefault("prefix", "")
			viper.SetDefault("web", false)
			viper.SetDefault("web_path", "")
			viper.SetDefault("admin_password", "")
			viper.SetDefault("admin_secret", "")
			viper.SetDefault("web_password", "") // Deprecated. Use admin_password
			viper.SetDefault("web_secret", "")   // Deprecated. Use admin_secret
			viper.SetDefault("max_channel_length", 255)
			viper.SetDefault("channel_prefix", "centrifugo")
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
			viper.SetDefault("sockjs_url", "//cdn.jsdelivr.net/sockjs/1.1/sockjs.min.js")

			viper.SetDefault("redis_connect_timeout", 1)
			viper.SetDefault("redis_write_timeout", 1)

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

			viper.SetEnvPrefix("centrifugo")

			bindEnvs := []string{
				"debug", "engine", "insecure", "insecure_api", "web", "admin", "admin_password", "admin_secret",
				"insecure_web", "insecure_admin", "secret", "connection_lifetime", "watch", "publish", "anonymous",
				"join_leave", "presence", "recover", "history_size", "history_lifetime", "history_drop_inactive",
				"redis_host", "redis_port", "redis_url",
			}
			for _, env := range bindEnvs {
				viper.BindEnv(env)
			}

			bindPFlags := []string{
				"port", "api_port", "admin_port", "address", "debug", "name", "admin", "insecure_admin", "web",
				"web_path", "insecure_web", "engine", "insecure", "insecure_api", "ssl", "ssl_cert", "ssl_key",
				"log_level", "log_file", "redis_host", "redis_port", "redis_password", "redis_db", "redis_url",
				"redis_api", "redis_pool", "redis_api_num_shards", "redis_master_name", "redis_sentinels",
			}
			for _, flag := range bindPFlags {
				viper.BindPFlag(flag, cmd.Flags().Lookup(flag))
			}

			viper.SetConfigFile(configFile)

			logger.INFO.Printf("Centrifugo version: %s", VERSION)
			logger.INFO.Printf("Process PID: %d", os.Getpid())

			absConfPath, err := filepath.Abs(configFile)
			if err != nil {
				logger.FATAL.Fatalln(err)
			}
			logger.INFO.Println("Config file search path:", absConfPath)

			err = viper.ReadInConfig()
			if err != nil {
				switch err.(type) {
				case viper.ConfigParseError:
					logger.FATAL.Fatalf("Error parsing configuration: %s\n", err)
				default:
					logger.WARN.Println("No config file found")
				}
			}

			setupLogging()

			if os.Getenv("GOMAXPROCS") == "" {
				if viper.IsSet("gomaxprocs") && viper.GetInt("gomaxprocs") > 0 {
					runtime.GOMAXPROCS(viper.GetInt("gomaxprocs"))
				} else {
					runtime.GOMAXPROCS(runtime.NumCPU())
				}
			}

			logger.INFO.Println("GOMAXPROCS:", runtime.GOMAXPROCS(0))

			c := newConfig()
			err = c.Validate()
			if err != nil {
				logger.FATAL.Fatalln(err)
			}

			app, err := libcentrifugo.NewApplication(c)
			if err != nil {
				logger.FATAL.Fatalln(err)
			}

			if c.Insecure {
				logger.WARN.Println("Running in INSECURE client mode")
			}
			if c.InsecureAPI {
				logger.WARN.Println("Running in INSECURE API mode")
			}
			if c.InsecureAdmin {
				logger.WARN.Println("Running in INSECURE admin mode")
			}

			var e libcentrifugo.Engine
			switch viper.GetString("engine") {
			case "memory":
				e = libcentrifugo.NewMemoryEngine(app)
			case "redis":
				masterName := viper.GetString("redis_master_name")
				sentinels := viper.GetString("redis_sentinels")
				if masterName != "" && sentinels == "" {
					logger.FATAL.Fatalf("Provide at least one Sentinel address")
				}

				sentinelAddrs := []string{}
				if sentinels != "" {
					for _, addr := range strings.Split(sentinels, ",") {
						addr := strings.TrimSpace(addr)
						if addr == "" {
							continue
						}
						if _, _, err := net.SplitHostPort(addr); err != nil {
							logger.FATAL.Fatalf("Malformed Sentinel address: %s", addr)
						}
						sentinelAddrs = append(sentinelAddrs, addr)
					}
				}

				if len(sentinelAddrs) > 0 && masterName == "" {
					logger.FATAL.Fatalln("Redis master name required when Sentinel used")
				}

				redisConf := &libcentrifugo.RedisEngineConfig{
					Host:           viper.GetString("redis_host"),
					Port:           viper.GetString("redis_port"),
					Password:       viper.GetString("redis_password"),
					DB:             viper.GetString("redis_db"),
					URL:            viper.GetString("redis_url"),
					PoolSize:       viper.GetInt("redis_pool"),
					API:            viper.GetBool("redis_api"),
					NumAPIShards:   viper.GetInt("redis_api_num_shards"),
					MasterName:     masterName,
					SentinelAddrs:  sentinelAddrs,
					ConnectTimeout: time.Duration(viper.GetInt("redis_connect_timeout")) * time.Second,
					ReadTimeout:    time.Duration(viper.GetInt("node_ping_interval")*3+1) * time.Second,
					WriteTimeout:   time.Duration(viper.GetInt("redis_write_timeout")) * time.Second,
				}
				e = libcentrifugo.NewRedisEngine(app, redisConf)
			default:
				logger.FATAL.Fatalln("Unknown engine: " + viper.GetString("engine"))
			}

			logger.INFO.Println("Engine:", viper.GetString("engine"))
			logger.TRACE.Printf("%v\n", viper.AllSettings())
			logger.INFO.Println("Use SSL:", viper.GetBool("ssl"))
			if viper.GetBool("ssl") {
				if viper.GetString("ssl_cert") == "" {
					logger.FATAL.Println("No SSL certificate provided")
					os.Exit(1)
				}
				if viper.GetString("ssl_key") == "" {
					logger.FATAL.Println("No SSL certificate key provided")
					os.Exit(1)
				}
			}
			app.SetEngine(e)
			err = app.Run()
			if err != nil {
				logger.FATAL.Fatalln(err)
			}

			go handleSignals(app)

			sockjsOpts := sockjs.DefaultOptions

			// Override sockjs url. It's important to use the same SockJS library version
			// on client and server sides, otherwise SockJS will report version mismatch
			// and won't work.
			sockjsURL := viper.GetString("sockjs_url")
			if sockjsURL != "" {
				logger.INFO.Println("SockJS url:", sockjsURL)
				sockjsOpts.SockJSURL = sockjsURL
			}
			if c.PingInterval < time.Second {
				logger.FATAL.Fatalln("Ping interval can not be less than one second.")
			}
			sockjsOpts.HeartbeatDelay = c.PingInterval

			webEnabled := viper.GetBool("web")

			var webFS http.FileSystem
			if webEnabled {
				webFS = assetFS()
			}

			adminEnabled := viper.GetBool("admin")
			if webEnabled {
				adminEnabled = true
			}

			clientPort := viper.GetString("port")

			apiPort := viper.GetString("api_port")
			if apiPort == "" {
				apiPort = clientPort
			}

			adminPort := viper.GetString("admin_port")
			if adminPort == "" {
				adminPort = clientPort
			}

			// portToHandlerFlags contains mapping between ports and handler flags
			// to serve on this port.
			portToHandlerFlags := map[string]libcentrifugo.HandlerFlag{}

			var portFlags libcentrifugo.HandlerFlag

			portFlags = portToHandlerFlags[clientPort]
			portFlags |= libcentrifugo.HandlerRawWS | libcentrifugo.HandlerSockJS
			portToHandlerFlags[clientPort] = portFlags

			portFlags = portToHandlerFlags[apiPort]
			portFlags |= libcentrifugo.HandlerAPI
			portToHandlerFlags[apiPort] = portFlags

			portFlags = portToHandlerFlags[adminPort]
			if adminEnabled {
				portFlags |= libcentrifugo.HandlerAdmin
			}
			if viper.GetBool("debug") {
				portFlags |= libcentrifugo.HandlerDebug
			}
			portToHandlerFlags[adminPort] = portFlags

			var wg sync.WaitGroup
			// Iterate over port to flags mapping and start HTTP servers
			// on separate ports serving handlers specified in flags.
			for handlerPort, handlerFlags := range portToHandlerFlags {
				muxOpts := libcentrifugo.MuxOptions{
					Prefix:        viper.GetString("prefix"),
					Admin:         adminEnabled,
					Web:           webEnabled,
					WebPath:       viper.GetString("web_path"),
					WebFS:         webFS,
					HandlerFlags:  handlerFlags,
					SockjsOptions: sockjsOpts,
				}
				mux := libcentrifugo.DefaultMux(app, muxOpts)

				addr := net.JoinHostPort(viper.GetString("address"), handlerPort)

				logger.INFO.Printf("Start serving %s endpoints on %s\n", handlerFlags, addr)
				wg.Add(1)
				go listenHTTP(mux, addr, useSSL, sslCert, sslKey, &wg)
			}
			wg.Wait()
		},
	}
	rootCmd.Flags().StringVarP(&port, "port", "p", "8000", "port to bind to")
	rootCmd.Flags().StringVarP(&address, "address", "a", "", "address to listen on")
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "debug mode - please, do not use it in production")
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	rootCmd.Flags().StringVarP(&name, "name", "n", "", "unique node name")
	rootCmd.Flags().BoolVarP(&admin, "admin", "", false, "Enable admin socket")
	rootCmd.Flags().BoolVarP(&web, "web", "w", false, "serve admin web interface application (warning: automatically enables admin socket)")
	rootCmd.Flags().StringVarP(&webPath, "web_path", "", "", "optional path to web interface application")
	rootCmd.Flags().StringVarP(&engn, "engine", "e", "memory", "engine to use: memory or redis")
	rootCmd.Flags().BoolVarP(&insecure, "insecure", "", false, "start in insecure client mode")
	rootCmd.Flags().BoolVarP(&insecureAPI, "insecure_api", "", false, "use insecure API mode")
	rootCmd.Flags().BoolVarP(&insecureWeb, "insecure_web", "", false, "use insecure web mode – no web password and web secret required for web interface (warning: automatically enables insecure_admin option)")
	rootCmd.Flags().BoolVarP(&insecureAdmin, "insecure_admin", "", false, "use insecure admin mode – no auth required for admin socket")
	rootCmd.Flags().BoolVarP(&useSSL, "ssl", "", false, "accept SSL connections. This requires an X509 certificate and a key file")
	rootCmd.Flags().StringVarP(&sslCert, "ssl_cert", "", "", "path to an X509 certificate file")
	rootCmd.Flags().StringVarP(&sslKey, "ssl_key", "", "", "path to an X509 certificate key")
	rootCmd.Flags().StringVarP(&apiPort, "api_port", "", "", "port to bind api endpoints to (optional until this is required by your deploy setup)")
	rootCmd.Flags().StringVarP(&adminPort, "admin_port", "", "", "port to bind admin endpoints to (optional until this is required by your deploy setup)")
	rootCmd.Flags().StringVarP(&logLevel, "log_level", "", "debug", "set the log level: trace, debug, info, error, critical, fatal or none")
	rootCmd.Flags().StringVarP(&logFile, "log_file", "", "", "optional log file - if not specified all logs go to STDOUT")
	rootCmd.Flags().StringVarP(&redisHost, "redis_host", "", "127.0.0.1", "redis host (Redis engine)")
	rootCmd.Flags().StringVarP(&redisPort, "redis_port", "", "6379", "redis port (Redis engine)")
	rootCmd.Flags().StringVarP(&redisPassword, "redis_password", "", "", "redis auth password (Redis engine)")
	rootCmd.Flags().StringVarP(&redisDB, "redis_db", "", "0", "redis database (Redis engine)")
	rootCmd.Flags().StringVarP(&redisURL, "redis_url", "", "", "redis connection URL (Redis engine)")
	rootCmd.Flags().BoolVarP(&redisAPI, "redis_api", "", false, "enable Redis API listener (Redis engine)")
	rootCmd.Flags().IntVarP(&redisPool, "redis_pool", "", 256, "Redis pool size (Redis engine)")
	rootCmd.Flags().IntVarP(&redisAPINumShards, "redis_api_num_shards", "", 0, "Number of shards for redis API queue (Redis engine)")
	rootCmd.Flags().StringVarP(&redisMasterName, "redis_master_name", "", "", "Name of Redis master Sentinel monitors (Redis engine)")
	rootCmd.Flags().StringVarP(&redisSentinels, "redis_sentinels", "", "", "Comma separated list of Sentinels (Redis engine)")

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
