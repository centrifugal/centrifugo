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

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/spf13/cobra"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/spf13/viper"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/gopkg.in/igm/sockjs-go.v2/sockjs"
	"github.com/centrifugal/centrifugo/libcentrifugo"
)

const LOG_FILE = "log_file"
const LOG_LEVEL = "log_level"
const DEBUG = "debug"
const ENGINE = "engine"
const INSECURE = "insecure"
const INSECURE_API = "insecure_api"
const WEB = "web"
const WEB_PATH = "web_path"
const PREFIX = "prefix"
const ADDRESS = "address"
const ADMIN_PASSWORD = "admin_password"
const ADMIN = "admin"
const ADMIN_SECRET = "admin_secret"
const INSECURE_WEB = "insecure_web"
const INSECURE_ADMIN = "insecure_admin"
const NAME = "name"
const MAX_CHANNEL_LENGTH = "max_channel_length"
const REDIS_CONNECT_TIMEOUT = "redis_connect_timeout"
const NODE_PING_INTERVAL = "node_ping_interval"
const CHANNEL_PREFIX = "channel_prefix"
const MESSAGE_SEND_TIMEOUT = "message_send_timeout"
const PING_INTERVAL = "ping_interval"
const ADMIN_PORT = "admin_port"
const NODE_METRICS_INTERVAL = "node_metrics_interval"
const CENTRIFUGO_PREFIX = "centrifugo"
const PORT = "port"
const STALE_CONNECTION_CLOSE_DELAY = "stale_connection_close_delay"
const REDIS_WRITE_TIMEOUT = "redis_write_timeout"
const REDIS_HOST = "redis_host"
const REDIS_PORT = "redis_port"
const SSL = "ssl"
const SSL_CERT = "ssl_cert"
const SSL_KEY = "ssl_key"
const API_PORT = "api_port"
const REDIS_PASSWORD = "redis_password"
const REDIS_DB = "redis_db"
const REDIS_URL = "redis_url"
const REDIS_API = "redis_api"
const REDIS_POOL = "redis_pool"
const REDIS_API_NUM_SHARDS = "redis_api_num_shards"
const REDIS_MASTER_NAME = "redis_master_name"
const REDIS_SENTINELS = "redis_sentinels"
const GO_MAXPROX = "gomaxprocs"
const SOCKJS_URL = "sockjs_url"
const SECRET = "secret"
const CONNECTION_TIMEOUT = "connection_lifetime"
const WATCH = "watch"
const PUBLISH = "publish"
const ANONYMOUS = "anonymous"
const JOIN_LEAVE = "join_leave"
const PRESENCE = "presence"
const RECOVER = "recover"
const HISTORY_SIZE = "history_size"
const HISTORY_LIFETIME = "history_lifetime"
const HISTORY_DROP_INACTIVE = "history_drop_inactive"
const NAMESPACES = "namespaces"

const ENGINE_MEMORY = "memory"
const ENGINE_REDIS = "redis"

const EMPTY = ""

func setupLogging() {
	logLevel, ok := logger.LevelMatches[strings.ToUpper(viper.GetString(LOG_LEVEL))]
	if !ok {
		logLevel = logger.LevelInfo
	}
	logger.SetLogThreshold(logLevel)
	logger.SetStdoutThreshold(logLevel)

	if viper.IsSet(LOG_FILE) && viper.GetString(LOG_FILE) != EMPTY {
		logger.SetLogFile(viper.GetString(LOG_FILE))
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
		Use:   EMPTY,
		Short: "Centrifugo",
		Long:  "Centrifugo. Real-time messaging (Websockets or SockJS) server in Go.",
		Run: func(cmd *cobra.Command, args []string) {

			viper.SetDefault(GO_MAXPROX, 0)
			viper.SetDefault(DEBUG, false)
			viper.SetDefault(PREFIX, EMPTY)
			viper.SetDefault(WEB, false)
			viper.SetDefault(WEB_PATH, EMPTY)
			viper.SetDefault(ADMIN_PASSWORD, EMPTY)
			viper.SetDefault(ADMIN_SECRET, EMPTY)
			viper.SetDefault(MAX_CHANNEL_LENGTH, 255)
			viper.SetDefault(CHANNEL_PREFIX, CENTRIFUGO_PREFIX)
			viper.SetDefault(NODE_PING_INTERVAL, 3)
			viper.SetDefault(MESSAGE_SEND_TIMEOUT, 0)
			viper.SetDefault(PING_INTERVAL, 25)
			viper.SetDefault(NODE_METRICS_INTERVAL, 60)
			viper.SetDefault(STALE_CONNECTION_CLOSE_DELAY, 25)
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
			viper.SetDefault(SOCKJS_URL, "//cdn.jsdelivr.net/sockjs/1.0/sockjs.min.js")

			viper.SetDefault(REDIS_CONNECT_TIMEOUT, 1)
			viper.SetDefault(REDIS_WRITE_TIMEOUT, 1)

			viper.SetDefault(SECRET, EMPTY)
			viper.SetDefault(CONNECTION_TIMEOUT, 0)
			viper.SetDefault(WATCH, false)
			viper.SetDefault(PUBLISH, false)
			viper.SetDefault(ANONYMOUS, false)
			viper.SetDefault(PRESENCE, false)
			viper.SetDefault(HISTORY_SIZE, 0)
			viper.SetDefault(HISTORY_LIFETIME, 0)
			viper.SetDefault(RECOVER, false)
			viper.SetDefault(HISTORY_DROP_INACTIVE, false)
			viper.SetDefault(NAMESPACES, EMPTY)

			viper.RegisterAlias(ADMIN_PASSWORD, "web_password")
			viper.RegisterAlias(ADMIN_SECRET, "web_secret")

			viper.SetEnvPrefix(CENTRIFUGO_PREFIX)

			bindEnvs := []string{DEBUG, ENGINE, INSECURE, INSECURE_API, WEB, ADMIN, ADMIN_PASSWORD, ADMIN_SECRET,
				INSECURE_WEB, INSECURE_ADMIN, SECRET, CONNECTION_TIMEOUT, WATCH, PUBLISH, ANONYMOUS, JOIN_LEAVE,
				PRESENCE, RECOVER, HISTORY_SIZE, HISTORY_LIFETIME, HISTORY_DROP_INACTIVE, REDIS_HOST, REDIS_PORT,
				REDIS_URL}

			for _, env := range bindEnvs {
				viper.BindEnv(env)
			}

			bindPFlags := []string{PORT, API_PORT, ADMIN_PORT, ADDRESS, DEBUG, NAME, ADMIN, INSECURE_ADMIN,
				WEB, WEB_PATH, INSECURE_WEB, ENGINE, INSECURE, INSECURE_API, SSL, SSL_CERT, SSL_KEY,
				LOG_LEVEL, LOG_FILE, REDIS_HOST, REDIS_PORT, REDIS_PASSWORD, REDIS_DB, REDIS_URL,
				REDIS_API, REDIS_POOL, REDIS_API_NUM_SHARDS, REDIS_MASTER_NAME, REDIS_SENTINELS}

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

			if os.Getenv("GOMAXPROCS") == EMPTY {
				if viper.IsSet(GO_MAXPROX) && viper.GetInt(GO_MAXPROX) > 0 {
					runtime.GOMAXPROCS(viper.GetInt(GO_MAXPROX))
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
			switch viper.GetString(ENGINE) {
			case ENGINE_MEMORY:
				e = libcentrifugo.NewMemoryEngine(app)
			case ENGINE_REDIS:
				masterName := viper.GetString(REDIS_MASTER_NAME)
				sentinels := viper.GetString(REDIS_SENTINELS)
				if masterName != EMPTY && sentinels == EMPTY {
					logger.FATAL.Fatalf("Provide at least one Sentinel address")
				}

				sentinelAddrs := []string{}
				if sentinels != EMPTY {
					for _, addr := range strings.Split(sentinels, ",") {
						addr := strings.TrimSpace(addr)
						if addr == EMPTY {
							continue
						}
						if _, _, err := net.SplitHostPort(addr); err != nil {
							logger.FATAL.Fatalf("Malformed Sentinel address: %s", addr)
						}
						sentinelAddrs = append(sentinelAddrs, addr)
					}
				}

				if len(sentinelAddrs) > 0 && masterName == EMPTY {
					logger.FATAL.Fatalln("Redis master name required when Sentinel used")
				}

				redisConf := &libcentrifugo.RedisEngineConfig{
					Host:           viper.GetString(REDIS_HOST),
					Port:           viper.GetString(REDIS_PORT),
					Password:       viper.GetString(REDIS_PASSWORD),
					DB:             viper.GetString(REDIS_DB),
					URL:            viper.GetString(REDIS_URL),
					PoolSize:       viper.GetInt(REDIS_POOL),
					API:            viper.GetBool(REDIS_API),
					NumAPIShards:   viper.GetInt(REDIS_API_NUM_SHARDS),
					MasterName:     masterName,
					SentinelAddrs:  sentinelAddrs,
					ConnectTimeout: time.Duration(viper.GetInt(REDIS_CONNECT_TIMEOUT)) * time.Second,
					ReadTimeout:    time.Duration(viper.GetInt(NODE_PING_INTERVAL)*3+1) * time.Second,
					WriteTimeout:   time.Duration(viper.GetInt("redis_write_timeout")) * time.Second,
				}
				e = libcentrifugo.NewRedisEngine(app, redisConf)
			default:
				logger.FATAL.Fatalln("Unknown engine: " + viper.GetString(ENGINE))
			}

			logger.INFO.Println("Engine:", viper.GetString(ENGINE))
			logger.DEBUG.Printf("%v\n", viper.AllSettings())
			logger.INFO.Println("Use SSL:", viper.GetBool(SSL))
			if viper.GetBool(SSL) {
				if viper.GetString(SSL_CERT) == EMPTY {
					logger.FATAL.Println("No SSL certificate provided")
					os.Exit(1)
				}
				if viper.GetString(SSL_KEY) == EMPTY {
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
			sockjsURL := viper.GetString(SOCKJS_URL)
			if sockjsURL != EMPTY {
				logger.INFO.Println("SockJS url:", sockjsURL)
				sockjsOpts.SockJSURL = sockjsURL
			}
			if c.PingInterval < time.Second {
				logger.FATAL.Fatalln("Ping interval can not be less than one second.")
			}
			sockjsOpts.HeartbeatDelay = c.PingInterval

			webEnabled := viper.GetBool(WEB)

			var webFS http.FileSystem
			if webEnabled {
				webFS = assetFS()
			}

			adminEnabled := viper.GetBool(ADMIN)
			if webEnabled {
				adminEnabled = true
			}

			clientPort := viper.GetString(PORT)

			apiPort := viper.GetString(API_PORT)
			if apiPort == EMPTY {
				apiPort = clientPort
			}

			adminPort := viper.GetString(ADMIN_PORT)
			if adminPort == EMPTY {
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
			if viper.GetBool(DEBUG) {
				portFlags |= libcentrifugo.HandlerDebug
			}
			portToHandlerFlags[adminPort] = portFlags

			var wg sync.WaitGroup
			// Iterate over port to flags mapping and start HTTP servers
			// on separate ports serving handlers specified in flags.
			for handlerPort, handlerFlags := range portToHandlerFlags {
				muxOpts := libcentrifugo.MuxOptions{
					Prefix:        viper.GetString(PREFIX),
					Admin:         adminEnabled,
					Web:           webEnabled,
					WebPath:       viper.GetString(WEB_PATH),
					WebFS:         webFS,
					HandlerFlags:  handlerFlags,
					SockjsOptions: sockjsOpts,
				}
				mux := libcentrifugo.DefaultMux(app, muxOpts)

				addr := net.JoinHostPort(viper.GetString(ADDRESS), handlerPort)

				logger.INFO.Printf("Start serving %s endpoints on %s\n", handlerFlags, addr)
				wg.Add(1)
				go listenHTTP(mux, addr, useSSL, sslCert, sslKey, &wg)
			}
			wg.Wait()
		},
	}
	rootCmd.Flags().StringVarP(&port, PORT, "p", "8000", "port to bind to")
	rootCmd.Flags().StringVarP(&address, ADDRESS, "a", EMPTY, "address to listen on")
	rootCmd.Flags().BoolVarP(&debug, DEBUG, "d", false, "debug mode - please, do not use it in production")
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	rootCmd.Flags().StringVarP(&name, NAME, "n", EMPTY, "unique node name")
	rootCmd.Flags().BoolVarP(&admin, ADMIN, EMPTY, false, "Enable admin socket")
	rootCmd.Flags().BoolVarP(&web, WEB, "w", false, "serve admin web interface application (warning: automatically enables admin socket)")
	rootCmd.Flags().StringVarP(&webPath, WEB_PATH, EMPTY, EMPTY, "optional path to web interface application")
	rootCmd.Flags().StringVarP(&engn, ENGINE, "e", ENGINE_MEMORY, "engine to use: memory or redis")
	rootCmd.Flags().BoolVarP(&insecure, INSECURE, EMPTY, false, "start in insecure client mode")
	rootCmd.Flags().BoolVarP(&insecureAPI, INSECURE_API, EMPTY, false, "use insecure API mode")
	rootCmd.Flags().BoolVarP(&insecureWeb, INSECURE_WEB, EMPTY, false, "use insecure web mode – no web password and web secret required for web interface (warning: automatically enables insecure_admin option)")
	rootCmd.Flags().BoolVarP(&insecureAdmin, INSECURE_ADMIN, EMPTY, false, "use insecure admin mode – no auth required for admin socket")
	rootCmd.Flags().BoolVarP(&useSSL, SSL, EMPTY, false, "accept SSL connections. This requires an X509 certificate and a key file")
	rootCmd.Flags().StringVarP(&sslCert, SSL_CERT, EMPTY, EMPTY, "path to an X509 certificate file")
	rootCmd.Flags().StringVarP(&sslKey, SSL_KEY, EMPTY, EMPTY, "path to an X509 certificate key")
	rootCmd.Flags().StringVarP(&apiPort, API_PORT, EMPTY, EMPTY, "port to bind api endpoints to (optional until this is required by your deploy setup)")
	rootCmd.Flags().StringVarP(&adminPort, ADMIN_PORT, EMPTY, EMPTY, "port to bind admin endpoints to (optional until this is required by your deploy setup)")
	rootCmd.Flags().StringVarP(&logLevel, LOG_LEVEL, EMPTY, "info", "set the log level: debug, info, error, critical, fatal or none")
	rootCmd.Flags().StringVarP(&logFile, LOG_FILE, EMPTY, EMPTY, "optional log file - if not specified all logs go to STDOUT")
	rootCmd.Flags().StringVarP(&redisHost, REDIS_HOST, EMPTY, "127.0.0.1", "redis host (Redis engine)")
	rootCmd.Flags().StringVarP(&redisPort, REDIS_PORT, EMPTY, "6379", "redis port (Redis engine)")
	rootCmd.Flags().StringVarP(&redisPassword, REDIS_PASSWORD, EMPTY, EMPTY, "redis auth password (Redis engine)")
	rootCmd.Flags().StringVarP(&redisDB, REDIS_DB, EMPTY, "0", "redis database (Redis engine)")
	rootCmd.Flags().StringVarP(&redisURL, REDIS_URL, EMPTY, EMPTY, "redis connection URL (Redis engine)")
	rootCmd.Flags().BoolVarP(&redisAPI, REDIS_API, EMPTY, false, "enable Redis API listener (Redis engine)")
	rootCmd.Flags().IntVarP(&redisPool, REDIS_POOL, EMPTY, 256, "Redis pool size (Redis engine)")
	rootCmd.Flags().IntVarP(&redisAPINumShards, REDIS_API_NUM_SHARDS, EMPTY, 0, "Number of shards for redis API queue (Redis engine)")
	rootCmd.Flags().StringVarP(&redisMasterName, REDIS_MASTER_NAME, EMPTY, EMPTY, "Name of Redis master Sentinel monitors (Redis engine)")
	rootCmd.Flags().StringVarP(&redisSentinels, REDIS_SENTINELS, EMPTY, EMPTY, "Comma separated list of Sentinels (Redis engine)")

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

func main() {
	Main()
}
