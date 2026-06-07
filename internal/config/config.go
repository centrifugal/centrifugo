// Package config contains Centrifugo Config and the code to load it.
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/config/envconfig"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/go-viper/mapstructure/v2"
	"github.com/hashicorp/go-envparse"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Config contains configuration options of Centrifugo.
type Config struct {
	// HTTP is a configuration for Centrifugo HTTP server.
	HTTP configtypes.HTTPServer `mapstructure:"http_server" json:"http_server" envconfig:"http_server" toml:"http_server" yaml:"http_server" doc:"Configures the main HTTP server: listen address, ports, and the internal endpoints port."`
	// Log is a configuration for logging.
	Log configtypes.Log `mapstructure:"log" json:"log" envconfig:"log" toml:"log" yaml:"log" doc:"Configures logging: level and optional log file."`
	// Engine is a configuration for Centrifugo engine. It's a handy combination of Broker and PresenceManager.
	// Currently only memory and redis engines are supported – both implement all the features. For more granular
	// control use `broker` and `presence_manager` options.
	Engine configtypes.Engine `mapstructure:"engine" json:"engine" envconfig:"engine" toml:"engine" yaml:"engine" doc:"Selects the engine - a combined broker and presence manager. Supports <<memory>> (default) and <<redis>>. For finer control use <<broker>> and <<presence_manager>> instead."`
	// Broker allows to configure a message broker to use. Broker is responsible for PUB/SUB functionality
	// and channel message history and idempotency cache.
	// By default, memory Broker is used. Memory broker is superfast, but it's not distributed and all
	// data stored in memory (thus lost after node restart). Redis Broker provides seamless horizontal
	// scalability, fault-tolerance, and persistence over Centrifugo restarts. Centrifugo also supports
	// Nats Broker which only implements at most once PUB/SUB semantics.
	Broker configtypes.Broker `mapstructure:"broker" json:"broker" envconfig:"broker" toml:"broker" yaml:"broker" doc:"Configures the message broker (PUB/SUB, history, idempotency cache) when set separately from the engine. Supports <<memory>> (default, not distributed, lost on restart), <<redis>> (scalable and persistent), and <<nats>> (at-most-once PUB/SUB only)."`
	// PresenceManager allows to configure a presence manager to use. Presence manager is responsible for
	// presence information storage and retrieval. By default, memory PresenceManager is used. Memory
	// PresenceManager is superfast, but it's not distributed. Redis PresenceManager provides a seamless
	// horizontal scalability.
	PresenceManager configtypes.PresenceManager `mapstructure:"presence_manager" json:"presence_manager" envconfig:"presence_manager" toml:"presence_manager" yaml:"presence_manager" doc:"Configures the presence manager when set separately from the engine. Supports <<memory>> (default, not distributed) and <<redis>> (scalable)."`
	// MapBroker allows to configure a map broker for synchronized keyed state channels.
	// When enabled, clients can use map subscription types for channels in namespaces
	// that have a map subscription_type.
	MapBroker configtypes.MapBroker `mapstructure:"map_broker" json:"map_broker" envconfig:"map_broker" toml:"map_broker" yaml:"map_broker" doc:"Configures the map broker for synchronized keyed-state channels. Used by namespaces whose subscription type is <<map>>."`
	// Controller is a configuration for custom Centrifugo Controller used for
	// cross-node communication in multi-node clusters.
	Controller configtypes.Controller `mapstructure:"controller" json:"controller" envconfig:"controller" toml:"controller" yaml:"controller" doc:"Configures the controller used for cross-node communication in multi-node clusters."`

	// SharedPoll contains configuration for shared poll subscriptions.
	SharedPoll configtypes.SharedPoll `mapstructure:"shared_poll" json:"shared_poll" envconfig:"shared_poll" toml:"shared_poll" yaml:"shared_poll" doc:"Configures shared poll subscriptions."`

	// Client contains real-time client connection related configuration.
	Client configtypes.Client `mapstructure:"client" json:"client" envconfig:"client" toml:"client" yaml:"client" doc:"Configures real-time client connections: auth, tokens, allowed origins, limits, proxies, and the insecure mode."`
	// Channel contains real-time channel related configuration.
	Channel configtypes.Channel `mapstructure:"channel" json:"channel" envconfig:"channel" toml:"channel" yaml:"channel" doc:"Configures channels and namespaces: separators, default options, and per-namespace behavior."`
	// RPC is a configuration for client RPC calls.
	RPC configtypes.RPC `mapstructure:"rpc" json:"rpc" envconfig:"rpc" toml:"rpc" yaml:"rpc" doc:"Configures client RPC calls and RPC namespaces."`
	// Proxies is an array of proxies with custom names for the more granular control of channel-related events
	// in different channel namespaces.
	Proxies configtypes.NamedProxies `mapstructure:"proxies" default:"[]" json:"proxies" envconfig:"proxies" yaml:"proxies" toml:"proxies" doc:"Defines named proxies that namespaces can reference to forward channel-related events to your backend."`

	// HttpAPI is a configuration for HTTP server API. It's enabled by default.
	HttpAPI configtypes.HttpAPI `mapstructure:"http_api" json:"http_api" envconfig:"http_api" toml:"http_api" yaml:"http_api" doc:"Configures the server HTTP API (enabled by default): API key, insecure mode, and whether it's exposed on the external port."`
	// GrpcAPI is a configuration for gRPC server API. It's disabled by default.
	GrpcAPI configtypes.GrpcAPI `mapstructure:"grpc_api" json:"grpc_api" envconfig:"grpc_api" toml:"grpc_api" yaml:"grpc_api" doc:"Configures the server gRPC API (disabled by default): enable flag, port, TLS, and auth."`

	// Consumers is a configuration for message queue consumers. For example, Centrifugo can consume
	// messages from PostgreSQL transactional outbox table, or from Kafka topics.
	Consumers configtypes.Consumers `mapstructure:"consumers" default:"[]" json:"consumers" envconfig:"consumers" toml:"consumers" yaml:"consumers" doc:"Defines message queue consumers that ingest commands from external sources such as a PostgreSQL outbox table or Kafka topics."`

	// WebSocket configuration. This transport is enabled by default.
	WebSocket configtypes.WebSocket `mapstructure:"websocket" json:"websocket" envconfig:"websocket" toml:"websocket" yaml:"websocket" doc:"Configures the bidirectional WebSocket transport (enabled by default): message size limits, compression, ping/pong, and write timeouts."`
	// SSE is a configuration for Server-Sent Events based bidirectional emulation transport.
	SSE configtypes.SSE `mapstructure:"sse" json:"sse" envconfig:"sse" toml:"sse" yaml:"sse" doc:"Configures the bidirectional Server-Sent Events (EventSource) transport, which relies on the emulation endpoint. Disabled by default."`
	// HTTPStream is a configuration for HTTP streaming based bidirectional emulation transport.
	HTTPStream configtypes.HTTPStream `mapstructure:"http_stream" json:"http_stream" envconfig:"http_stream" toml:"http_stream" yaml:"http_stream" doc:"Configures the bidirectional HTTP-streaming transport, which relies on the emulation endpoint. Disabled by default."`
	// WebTransport is a configuration for WebTransport transport. EXPERIMENTAL.
	WebTransport configtypes.WebTransport `mapstructure:"webtransport" json:"webtransport" envconfig:"webtransport" toml:"webtransport" yaml:"webtransport" doc:"Configures the bidirectional WebTransport (HTTP/3) transport. Experimental and disabled by default."`
	// UniSSE is a configuration for unidirectional Server-Sent Events transport.
	UniSSE configtypes.UniSSE `mapstructure:"uni_sse" json:"uni_sse" envconfig:"uni_sse" toml:"uni_sse" yaml:"uni_sse" doc:"Configures the unidirectional Server-Sent Events (EventSource) transport. Disabled by default."`
	// UniHTTPStream is a configuration for unidirectional HTTP streaming transport.
	UniHTTPStream configtypes.UniHTTPStream `mapstructure:"uni_http_stream" json:"uni_http_stream" envconfig:"uni_http_stream" toml:"uni_http_stream" yaml:"uni_http_stream" doc:"Configures the unidirectional HTTP-streaming transport. Disabled by default."`
	// UniWS is a configuration for unidirectional WebSocket transport.
	UniWS configtypes.UniWebSocket `mapstructure:"uni_websocket" json:"uni_websocket" envconfig:"uni_websocket" toml:"uni_websocket" yaml:"uni_websocket" doc:"Configures the unidirectional WebSocket transport. Disabled by default."`
	// UniGRPC is a configuration for unidirectional gRPC transport.
	UniGRPC configtypes.UniGRPC `mapstructure:"uni_grpc" json:"uni_grpc" envconfig:"uni_grpc" toml:"uni_grpc" yaml:"uni_grpc" doc:"Configures the unidirectional gRPC transport: enable flag, port, and TLS. Disabled by default."`
	// Emulation endpoint is enabled automatically when at least one bidirectional emulation transport
	// is configured (SSE or HTTP Stream).
	Emulation configtypes.Emulation `mapstructure:"emulation" json:"emulation" envconfig:"emulation" toml:"emulation" yaml:"emulation" doc:"Configures the emulation endpoint used by bidirectional SSE and HTTP-streaming transports. Enabled automatically when one of those transports is enabled."`
	// Admin web UI configuration.
	Admin configtypes.Admin `mapstructure:"admin" json:"admin" envconfig:"admin" toml:"admin" yaml:"admin" doc:"Configures the admin web UI and its API endpoints."`
	// Prometheus metrics configuration.
	Prometheus configtypes.Prometheus `mapstructure:"prometheus" json:"prometheus" envconfig:"prometheus" toml:"prometheus" yaml:"prometheus" doc:"Configures the Prometheus metrics endpoint."`
	// Health check endpoint configuration.
	Health configtypes.Health `mapstructure:"health" json:"health" envconfig:"health" toml:"health" yaml:"health" doc:"Configures the health check endpoint."`
	// Swagger documentation (for server HTTP API) configuration.
	Swagger configtypes.Swagger `mapstructure:"swagger" json:"swagger" envconfig:"swagger" toml:"swagger" yaml:"swagger" doc:"Configures the Swagger UI endpoint describing the server HTTP API."`
	// Debug helps to enable Go profiling endpoints.
	Debug configtypes.Debug `mapstructure:"debug" json:"debug" envconfig:"debug" toml:"debug" yaml:"debug" doc:"Configures Go pprof profiling endpoints. For debugging only - do not expose publicly."`
	// Dev is a configuration for development page with simple Centrifugo client connection test.
	Dev configtypes.Dev `mapstructure:"dev" json:"dev" envconfig:"dev" toml:"dev" yaml:"dev" expose:"-"`
	// Init is a configuration for connection initialization endpoint.
	Init configtypes.ConnInit `mapstructure:"init" json:"init" envconfig:"init" toml:"init" yaml:"init" doc:"Configures the connection initialization endpoint."`

	// OpenTelemetry is a configuration for OpenTelemetry tracing.
	OpenTelemetry configtypes.OpenTelemetry `mapstructure:"opentelemetry" json:"opentelemetry" envconfig:"opentelemetry" toml:"opentelemetry" yaml:"opentelemetry" doc:"Configures OpenTelemetry tracing export."`
	// Graphite is a configuration for export metrics to Graphite.
	Graphite configtypes.Graphite `mapstructure:"graphite" json:"graphite" envconfig:"graphite" toml:"graphite" yaml:"graphite" doc:"Configures exporting metrics to Graphite."`
	// UsageStats is a configuration for usage stats sending.
	UsageStats configtypes.UsageStats `mapstructure:"usage_stats" json:"usage_stats" envconfig:"usage_stats" toml:"usage_stats" yaml:"usage_stats" doc:"Configures anonymous usage statistics reporting. Set <<enabled>> to <<false>> to opt out."`
	// Node is a configuration for Centrifugo Node as part of cluster.
	Node configtypes.Node `mapstructure:"node" json:"node" envconfig:"node" toml:"node" yaml:"node" doc:"Configures this node as part of a Centrifugo cluster: node name and inter-node info settings."`
	// Shutdown is a configuration for graceful shutdown.
	Shutdown configtypes.Shutdown `mapstructure:"shutdown" json:"shutdown" envconfig:"shutdown" toml:"shutdown" yaml:"shutdown" doc:"Configures graceful shutdown behavior, such as the shutdown termination timeout."`

	// PidFile is a path to write a file with Centrifugo process PID.
	PidFile string `mapstructure:"pid_file" json:"pid_file" envconfig:"pid_file" toml:"pid_file" yaml:"pid_file" expose:"full" doc:"Path of a file to write the Centrifugo process PID to. Leave empty to disable."`
	// EnableUnreleasedFeatures enables unreleased features. These features are not stable and may be removed even
	// in minor release update. Evaluate and share feedback if you find some feature useful and want it to be stabilized.
	EnableUnreleasedFeatures bool `mapstructure:"enable_unreleased_features" json:"enable_unreleased_features" envconfig:"enable_unreleased_features" toml:"enable_unreleased_features" yaml:"enable_unreleased_features" doc:"Enables unstable, unreleased features. These may change or be removed even in a minor release - do not rely on them in production."`
}

type Meta struct {
	FileNotFound        bool
	UnknownKeys         []string
	UnknownEnvs         []string
	KnownEnvVars        map[string]envconfig.VarInfo
	DeprecationWarnings []string
}

func DefineFlags(rootCmd *cobra.Command) {
	rootCmd.Flags().StringP("pid_file", "", "", "optional path to create PID file")
	rootCmd.Flags().StringP("http_server.address", "a", "", "interface address to listen on")
	rootCmd.Flags().StringP("http_server.port", "p", "8000", "port to bind HTTP server to")
	rootCmd.Flags().StringP("http_server.internal_address", "", "", "custom interface address to listen on for internal endpoints")
	rootCmd.Flags().StringP("http_server.internal_port", "", "", "custom port for internal endpoints")
	rootCmd.Flags().StringP("engine.type", "", "memory", "engine to use: memory or redis")
	rootCmd.Flags().BoolP("broker.enabled", "", false, "enable broker")
	rootCmd.Flags().StringP("broker.type", "", "memory", "broker to use: memory, redis or nats")
	rootCmd.Flags().BoolP("presence_manager.enabled", "", false, "enable presence manager")
	rootCmd.Flags().StringP("presence_manager.type", "", "memory", "presence manager to use: memory or redis")
	rootCmd.Flags().StringP("map_broker.type", "", "memory", "map broker to use: memory, redis or postgres")
	rootCmd.Flags().StringP("log.level", "", "info", "set the log level: trace, debug, info, warn, error, fatal or none")
	rootCmd.Flags().StringP("log.file", "", "", "optional log file - if not specified logs go to STDOUT")
	rootCmd.Flags().BoolP("debug.enabled", "", false, "enable debug endpoints")
	rootCmd.Flags().BoolP("admin.enabled", "", false, "enable admin web interface")
	rootCmd.Flags().BoolP("admin.external", "", false, "expose admin web interface on external port")
	rootCmd.Flags().BoolP("prometheus.enabled", "", false, "enable Prometheus metrics endpoint")
	rootCmd.Flags().BoolP("swagger.enabled", "", false, "enable Swagger UI endpoint describing server HTTP API")
	rootCmd.Flags().BoolP("health.enabled", "", false, "enable health check endpoint")
	rootCmd.Flags().BoolP("dev.enabled", "", false, "enable dev endpoint (must be used only in development)")
	rootCmd.Flags().BoolP("uni_websocket.enabled", "", false, "enable unidirectional websocket endpoint")
	rootCmd.Flags().BoolP("uni_sse.enabled", "", false, "enable unidirectional SSE (EventSource) endpoint")
	rootCmd.Flags().BoolP("uni_http_stream.enabled", "", false, "enable unidirectional HTTP-streaming endpoint")
	rootCmd.Flags().BoolP("sse.enabled", "", false, "enable bidirectional SSE (EventSource) endpoint (with emulation layer)")
	rootCmd.Flags().BoolP("http_stream.enabled", "", false, "enable bidirectional HTTP-streaming endpoint (with emulation layer)")
	rootCmd.Flags().BoolP("init.enabled", "", false, "enable connection init endpoint")
	rootCmd.Flags().BoolP("client.insecure", "", false, "start in insecure client mode")
	rootCmd.Flags().BoolP("http_api.insecure", "", false, "use insecure API mode")
	rootCmd.Flags().BoolP("http_api.external", "", false, "expose API handler on external port")
	rootCmd.Flags().BoolP("admin.insecure", "", false, "use insecure admin mode – no auth required for admin socket")
	rootCmd.Flags().BoolP("grpc_api.enabled", "", false, "enable GRPC API server")
	rootCmd.Flags().IntP("grpc_api.port", "", 10000, "port to bind GRPC API server to")
	rootCmd.Flags().BoolP("uni_grpc.enabled", "", false, "enable unidirectional GRPC endpoint")
	rootCmd.Flags().IntP("uni_grpc.port", "", 11000, "port to bind unidirectional GRPC server to")
}

func GetConfig(cmd *cobra.Command, configFile string) (Config, Meta, error) {
	v := viper.NewWithOptions(viper.WithDecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		configtypes.StringToDurationHookFunc(),
		configtypes.StringToPEMDataHookFunc(),
		configtypes.StringToMapStringStringHookFunc(),
		configtypes.StringToStringKeyValuesHookFunc(),
	)))

	if cmd != nil {
		bindPFlags := []string{
			"pid_file", "http_server.port", "http_server.address", "http_server.internal_port",
			"http_server.internal_address", "log.level", "log.file", "engine.type", "broker.enabled", "broker.type",
			"presence_manager.enabled", "presence_manager.type", "map_broker.type",
			"debug.enabled", "admin.enabled", "admin.external",
			"admin.insecure", "client.insecure", "http_api.insecure", "http_api.external", "prometheus.enabled",
			"health.enabled", "grpc_api.enabled", "grpc_api.port", "uni_grpc.enabled", "uni_grpc.port",
			"uni_websocket.enabled", "uni_sse.enabled", "uni_http_stream.enabled", "sse.enabled", "http_stream.enabled",
			"swagger.enabled", "dev.enabled", "init.enabled",
		}
		for _, flag := range bindPFlags {
			_ = v.BindPFlag(flag, cmd.Flags().Lookup(flag))
		}
	}

	meta := Meta{}

	if configFile != "" {
		v.SetConfigFile(configFile)
		err := v.ReadInConfig()
		if err != nil {
			var configFileNotFoundError *os.PathError
			if errors.As(err, &configFileNotFoundError) {
				meta.FileNotFound = true
			} else {
				return Config{}, Meta{}, fmt.Errorf("error reading config file %s: %w", configFile, err)
			}
		}
	}

	conf := &Config{}

	err := v.Unmarshal(conf)
	if err != nil {
		return Config{}, Meta{}, fmt.Errorf("error unmarshalling config: %w", err)
	}

	var flagsSet []string
	if cmd != nil {
		cmd.Flags().Visit(func(f *pflag.Flag) {
			if !slices.Contains(flagsSet, f.Name) {
				flagsSet = append(flagsSet, f.Name) // pid_file, http_server.port, etc.
			}
		})
	}

	knownEnvVars := map[string]envconfig.VarInfo{}
	varInfo, err := envconfig.ProcessWithoutNestedKeys("CENTRIFUGO", conf, flagsSet)
	if err != nil {
		return Config{}, Meta{}, fmt.Errorf("error processing env: %w", err)
	}
	extendKnownEnvVars(knownEnvVars, varInfo)

	for i, item := range conf.Channel.Namespaces {
		varInfo, err = envconfig.Process("CENTRIFUGO_CHANNEL_NAMESPACES_"+configtypes.NameForEnv(item.Name), &item)
		if err != nil {
			return Config{}, Meta{}, fmt.Errorf("error processing env namespaces: %w", err)
		}
		conf.Channel.Namespaces[i] = item
		extendKnownEnvVars(knownEnvVars, varInfo)
	}

	for i, item := range conf.RPC.Namespaces {
		varInfo, err = envconfig.Process("CENTRIFUGO_RPC_NAMESPACES_"+configtypes.NameForEnv(item.Name), &item)
		if err != nil {
			return Config{}, Meta{}, fmt.Errorf("error processing env rpc namespaces: %w", err)
		}
		conf.RPC.Namespaces[i] = item
		extendKnownEnvVars(knownEnvVars, varInfo)
	}

	for i, item := range conf.Proxies {
		varInfo, err = envconfig.Process("CENTRIFUGO_PROXIES_"+configtypes.NameForEnv(item.Name), &item)
		if err != nil {
			return Config{}, Meta{}, fmt.Errorf("error processing env named proxies: %w", err)
		}
		conf.Proxies[i] = item
		extendKnownEnvVars(knownEnvVars, varInfo)
	}

	for i, item := range conf.Consumers {
		varInfo, err = envconfig.Process("CENTRIFUGO_CONSUMERS_"+configtypes.NameForEnv(item.Name), &item)
		if err != nil {
			return Config{}, Meta{}, fmt.Errorf("error processing env consumers: %w", err)
		}
		conf.Consumers[i] = item
		extendKnownEnvVars(knownEnvVars, varInfo)
	}

	meta.UnknownKeys = findUnknownKeys(v.AllSettings(), conf, "")
	meta.UnknownEnvs = checkEnvironmentVars(knownEnvVars)
	meta.KnownEnvVars = knownEnvVars

	cfg := *conf
	cfg, deprecationWarnings := applyConfigMigrations(cfg)
	meta.DeprecationWarnings = deprecationWarnings

	return cfg, meta, nil
}

func extendKnownEnvVars(knownEnvVars map[string]envconfig.VarInfo, varInfo []envconfig.VarInfo) {
	for _, info := range varInfo {
		knownEnvVars[info.Key] = info
	}
}

// findValidKeys recursively finds valid keys in a struct, including embedded structs
func findValidKeys(typ reflect.Type, validKeys map[string]reflect.StructField) {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("mapstructure")

		if tag != "" && tag != ",squash" {
			// Normal field, add it to validKeys.
			validKeys[tag] = field
		} else if field.Anonymous && strings.Contains(tag, "squash") {
			// Handle embedded fields with "squash".
			embeddedType := field.Type
			if embeddedType.Kind() == reflect.Ptr {
				embeddedType = embeddedType.Elem()
			}
			if embeddedType.Kind() == reflect.Struct {
				// Recursively process the embedded struct
				findValidKeys(embeddedType, validKeys)
			}
		}
	}
}

func findUnknownKeys(data map[string]interface{}, configStruct interface{}, parentKey string) []string {
	var unknownKeys []string
	val := reflect.ValueOf(configStruct)

	if val.Kind() == reflect.Ptr && val.IsNil() {
		// Create an instance if the struct pointer is nil to avoid panic.
		val = reflect.New(val.Type().Elem())
	}

	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()

	// Build a set of valid keys from the struct's mapstructure tags, including embedded structs.
	validKeys := make(map[string]reflect.StructField)
	findValidKeys(typ, validKeys)

	// Check each key in the map to see if it's in the valid keys set
	for key, value := range data {
		if field, exists := validKeys[key]; exists {
			fieldValue := val.FieldByName(field.Name)

			if (fieldValue.Kind() == reflect.Struct || (fieldValue.Kind() == reflect.Ptr && fieldValue.Type().Elem().Kind() == reflect.Struct)) && !field.Anonymous {
				if nestedMap, ok := value.(map[string]interface{}); ok {
					// Handle pointers to structs specifically
					if fieldValue.Kind() == reflect.Ptr && fieldValue.IsNil() {
						fieldValue.Set(reflect.New(fieldValue.Type().Elem())) // Create new struct if nil
					}
					nestedStruct := fieldValue.Interface()
					if fieldValue.Kind() == reflect.Ptr {
						nestedStruct = fieldValue.Elem().Interface()
					}
					unknownKeys = append(unknownKeys, findUnknownKeys(nestedMap, nestedStruct, appendKeyPath(parentKey, key))...)
				}
			} else if fieldValue.Kind() == reflect.Slice {
				// Handle each element in the slice if it is a map
				if slice, ok := value.([]interface{}); ok {
					for i, elem := range slice {
						if elemMap, ok := elem.(map[string]interface{}); ok {
							elementType := fieldValue.Type().Elem()
							if elementType.Kind() == reflect.Ptr {
								elementType = elementType.Elem()
							}
							if elementType.Kind() == reflect.Struct {
								nestedStruct := reflect.New(elementType).Interface()
								unknownKeys = append(unknownKeys, findUnknownKeys(elemMap, nestedStruct, appendKeyPath(appendKeyPath(parentKey, key), fmt.Sprintf("[%d]", i)))...)
							}
						}
					}
				}
			}
		} else {
			unknownKeys = append(unknownKeys, appendKeyPath(parentKey, key))
		}
	}

	return unknownKeys
}

func appendKeyPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func checkEnvironmentVars(knownEnvVars map[string]envconfig.VarInfo) []string {
	var unknownEnvs []string
	envPrefix := "CENTRIFUGO_"
	envVars := os.Environ()

	for _, envVar := range envVars {
		kv, err := envparse.Parse(strings.NewReader(envVar))
		if err != nil {
			continue
		}
		for envKey := range kv {
			if !strings.HasPrefix(envKey, envPrefix) {
				continue
			}
			// Kubernetes automatically adds some variables which are not used by Centrifugo
			// itself. We skip warnings about them.
			if isKubernetesEnvVar(envKey) {
				continue
			}
			// Centrifugo can extrapolate ${CENTRIFUGO_VAR_XXX} from env inside MapStringString type.
			if isCustomEnvVar(envKey) {
				continue
			}
			if _, ok := knownEnvVars[envKey]; !ok {
				unknownEnvs = append(unknownEnvs, envKey)
			}
		}
	}
	return unknownEnvs
}

// k8sEnvRegex is used to filter out Kubernetes-injected environment variables.
// Centrifugo automatically scans environment variables starting with CENTRIFUGO_ and converts them to configuration options.
// However, Kubernetes automatically injects environment variables, so this regular expression filters out Kubernetes-injected environment variables.
// See isKubernetesEnvVar function for usage.
var k8sEnvRegex = regexp.MustCompile(`^CENTRIFUGO(?:_[A-Z0-9_]+)?_(PORT|SERVICE_)`)

func isKubernetesEnvVar(envKey string) bool {
	return k8sEnvRegex.MatchString(envKey)
}

func isCustomEnvVar(envKey string) bool {
	// Custom environment variables should start with "CENTRIFUGO_VAR_" prefix.
	// This function checks if the environment variable is a custom one.
	return strings.HasPrefix(envKey, "CENTRIFUGO_VAR_")
}

// DefaultConfig is a helper to be used in tests.
func DefaultConfig() Config {
	conf, _, err := GetConfig(nil, "")
	if err != nil {
		panic("error during getting default config: " + err.Error())
	}
	return conf
}
