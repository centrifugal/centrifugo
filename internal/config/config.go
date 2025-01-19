// Package config contains Centrifugo Config and the code to load it.
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/config/envconfig"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/go-viper/mapstructure/v2"
	"github.com/hashicorp/go-envparse"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Config struct {
	// HTTP is a configuration for Centrifugo HTTP server.
	HTTP configtypes.HTTPServer `mapstructure:"http_server" json:"http_server" envconfig:"http_server" toml:"http_server" yaml:"http_server"`
	// Log is a configuration for logging.
	Log configtypes.Log `mapstructure:"log" json:"log" envconfig:"log" toml:"log" yaml:"log"`
	// Engine is a configuration for Centrifugo engine. It's a handy combination of Broker and PresenceManager.
	// Currently only memory and redis engines are supported – both implement all the features. For more granular
	// control use Broker and PresenceManager options.
	Engine configtypes.Engine `mapstructure:"engine" json:"engine" envconfig:"engine" toml:"engine" yaml:"engine"`
	// Broker allows to configure a message broker to use. Broker is responsible for PUB/SUB functionality
	// and channel message history and idempotency cache .
	// By default, memory Broker is used. Memory broker is superfast, but it's not distributed and all
	// data stored in memory (thus lost after node restart). Redis Broker provides seamless horizontal
	// scalability, fault-tolerance, and persistence over Centrifugo restarts. Centrifugo also supports
	// Nats Broker which only implements at most once PUB/SUB semantics.
	Broker configtypes.Broker `mapstructure:"broker" json:"broker" envconfig:"broker" toml:"broker" yaml:"broker"`
	// PresenceManager allows to configure a presence manager to use. Presence manager is responsible for
	// presence information storage and retrieval. By default, memory PresenceManager is used. Memory
	// PresenceManager is superfast, but it's not distributed. Redis PresenceManager provides a seamless
	// horizontal scalability.
	PresenceManager configtypes.PresenceManager `mapstructure:"presence_manager" json:"presence_manager" envconfig:"presence_manager" toml:"presence_manager" yaml:"presence_manager"`

	// Client contains real-time client connection related configuration.
	Client configtypes.Client `mapstructure:"client" json:"client" envconfig:"client" toml:"client" yaml:"client"`
	// Channel contains real-time channel related configuration.
	Channel configtypes.Channel `mapstructure:"channel" json:"channel" envconfig:"channel" toml:"channel" yaml:"channel"`
	// RPC is a configuration for client RPC calls.
	RPC configtypes.RPC `mapstructure:"rpc" json:"rpc" envconfig:"rpc" toml:"rpc" yaml:"rpc"`
	// Proxies is an array of proxies with custom names for the more granular control of channel-related events
	// in different channel namespaces.
	Proxies configtypes.NamedProxies `mapstructure:"proxies" default:"[]" json:"proxies" envconfig:"proxies" yaml:"proxies" toml:"proxies"`

	// HttpAPI is a configuration for HTTP server API. It's enabled by default.
	HttpAPI configtypes.HttpAPI `mapstructure:"http_api" json:"http_api" envconfig:"http_api" toml:"http_api" yaml:"http_api"`
	// GrpcAPI is a configuration for gRPC server API. It's disabled by default.
	GrpcAPI configtypes.GrpcAPI `mapstructure:"grpc_api" json:"grpc_api" envconfig:"grpc_api" toml:"grpc_api" yaml:"grpc_api"`

	// Consumers is a configuration for message queue consumers. For example, Centrifugo can consume
	// messages from PostgreSQL transactional outbox table, or from Kafka topics.
	Consumers configtypes.Consumers `mapstructure:"consumers" default:"[]" json:"consumers" envconfig:"consumers" toml:"consumers" yaml:"consumers"`

	// WebSocket configuration. This transport is enabled by default.
	WebSocket configtypes.WebSocket `mapstructure:"websocket" json:"websocket" envconfig:"websocket" toml:"websocket" yaml:"websocket"`
	// SSE is a configuration for Server-Sent Events based bidirectional emulation transport.
	SSE configtypes.SSE `mapstructure:"sse" json:"sse" envconfig:"sse" toml:"sse" yaml:"sse"`
	// HTTPStream is a configuration for HTTP streaming based bidirectional emulation transport.
	HTTPStream configtypes.HTTPStream `mapstructure:"http_stream" json:"http_stream" envconfig:"http_stream" toml:"http_stream" yaml:"http_stream"`
	// WebTransport is a configuration for WebTransport transport. EXPERIMENTAL.
	WebTransport configtypes.WebTransport `mapstructure:"webtransport" json:"webtransport" envconfig:"webtransport" toml:"webtransport" yaml:"webtransport"`
	// UniSSE is a configuration for unidirectional Server-Sent Events transport.
	UniSSE configtypes.UniSSE `mapstructure:"uni_sse" json:"uni_sse" envconfig:"uni_sse" toml:"uni_sse" yaml:"uni_sse"`
	// UniHTTPStream is a configuration for unidirectional HTTP streaming transport.
	UniHTTPStream configtypes.UniHTTPStream `mapstructure:"uni_http_stream" json:"uni_http_stream" envconfig:"uni_http_stream" toml:"uni_http_stream" yaml:"uni_http_stream"`
	// UniWS is a configuration for unidirectional WebSocket transport.
	UniWS configtypes.UniWebSocket `mapstructure:"uni_websocket" json:"uni_websocket" envconfig:"uni_websocket" toml:"uni_websocket" yaml:"uni_websocket"`
	// UniGRPC is a configuration for unidirectional gRPC transport.
	UniGRPC configtypes.UniGRPC `mapstructure:"uni_grpc" json:"uni_grpc" envconfig:"uni_grpc" toml:"uni_grpc" yaml:"uni_grpc"`
	// Emulation endpoint is enabled automatically when at least one bidirectional emulation transport
	// is configured (SSE or HTTP Stream).
	Emulation configtypes.Emulation `mapstructure:"emulation" json:"emulation" envconfig:"emulation" toml:"emulation" yaml:"emulation"`
	// Admin web UI configuration.
	Admin configtypes.Admin `mapstructure:"admin" json:"admin" envconfig:"admin" toml:"admin" yaml:"admin"`
	// Prometheus metrics configuration.
	Prometheus configtypes.Prometheus `mapstructure:"prometheus" json:"prometheus" envconfig:"prometheus" toml:"prometheus" yaml:"prometheus"`
	// Health check endpoint configuration.
	Health configtypes.Health `mapstructure:"health" json:"health" envconfig:"health" toml:"health" yaml:"health"`
	// Swagger documentation (for server HTTP API) configuration.
	Swagger configtypes.Swagger `mapstructure:"swagger" json:"swagger" envconfig:"swagger" toml:"swagger" yaml:"swagger"`
	// Debug helps to enable Go profiling endpoints.
	Debug configtypes.Debug `mapstructure:"debug" json:"debug" envconfig:"debug" toml:"debug" yaml:"debug"`

	// OpenTelemetry is a configuration for OpenTelemetry tracing.
	OpenTelemetry configtypes.OpenTelemetry `mapstructure:"opentelemetry" json:"opentelemetry" envconfig:"opentelemetry" toml:"opentelemetry" yaml:"opentelemetry"`
	// Graphite is a configuration for export metrics to Graphite.
	Graphite configtypes.Graphite `mapstructure:"graphite" json:"graphite" envconfig:"graphite" toml:"graphite" yaml:"graphite"`
	// UsageStats is a configuration for usage stats sending.
	UsageStats configtypes.UsageStats `mapstructure:"usage_stats" json:"usage_stats" envconfig:"usage_stats" toml:"usage_stats" yaml:"usage_stats"`
	// Node is a configuration for Centrifugo Node as part of cluster.
	Node configtypes.Node `mapstructure:"node" json:"node" envconfig:"node" toml:"node" yaml:"node"`
	// Shutdown is a configuration for graceful shutdown.
	Shutdown configtypes.Shutdown `mapstructure:"shutdown" json:"shutdown" envconfig:"shutdown" toml:"shutdown" yaml:"shutdown"`

	// PidFile is a path to write a file with Centrifugo process PID.
	PidFile string `mapstructure:"pid_file" json:"pid_file" envconfig:"pid_file" toml:"pid_file" yaml:"pid_file"`
	// EnableUnreleasedFeatures enables unreleased features. These features are not stable and may be removed even
	// in minor release update. Evaluate and share feedback if you find some feature useful and want it to be stabilized.
	EnableUnreleasedFeatures bool `mapstructure:"enable_unreleased_features" json:"enable_unreleased_features" envconfig:"enable_unreleased_features" toml:"enable_unreleased_features" yaml:"enable_unreleased_features"`
}

type Meta struct {
	FileNotFound bool
	UnknownKeys  []string
	UnknownEnvs  []string
	KnownEnvVars map[string]envconfig.VarInfo
}

func DefineFlags(rootCmd *cobra.Command) {
	rootCmd.Flags().StringP("pid_file", "", "", "optional path to create PID file")
	rootCmd.Flags().StringP("http_server.address", "a", "", "interface address to listen on")
	rootCmd.Flags().StringP("http_server.port", "p", "8000", "port to bind HTTP server to")
	rootCmd.Flags().StringP("http_server.internal_address", "", "", "custom interface address to listen on for internal endpoints")
	rootCmd.Flags().StringP("http_server.internal_port", "", "", "custom port for internal endpoints")
	rootCmd.Flags().StringP("engine.type", "", "memory", "broker to use: ex. redis")
	rootCmd.Flags().BoolP("broker.enabled", "", false, "enable broker")
	rootCmd.Flags().StringP("broker.type", "", "memory", "broker to use: ex. redis")
	rootCmd.Flags().BoolP("presence_manager.enabled", "", false, "enable presence manager")
	rootCmd.Flags().StringP("presence_manager.type", "", "memory", "presence manager to use: ex. redis")
	rootCmd.Flags().StringP("log.level", "", "info", "set the log level: trace, debug, info, error, fatal or none")
	rootCmd.Flags().StringP("log.file", "", "", "optional log file - if not specified logs go to STDOUT")
	rootCmd.Flags().BoolP("debug.enabled", "", false, "enable debug endpoints")
	rootCmd.Flags().BoolP("admin.enabled", "", false, "enable admin web interface")
	rootCmd.Flags().BoolP("admin.external", "", false, "expose admin web interface on external port")
	rootCmd.Flags().BoolP("prometheus.enabled", "", false, "enable Prometheus metrics endpoint")
	rootCmd.Flags().BoolP("swagger.enabled", "", false, "enable Swagger UI endpoint describing server HTTP API")
	rootCmd.Flags().BoolP("health.enabled", "", false, "enable health check endpoint")
	rootCmd.Flags().BoolP("uni_websocket.enabled", "", false, "enable unidirectional websocket endpoint")
	rootCmd.Flags().BoolP("uni_sse.enabled", "", false, "enable unidirectional SSE (EventSource) endpoint")
	rootCmd.Flags().BoolP("uni_http_stream.enabled", "", false, "enable unidirectional HTTP-streaming endpoint")
	rootCmd.Flags().BoolP("sse.enabled", "", false, "enable bidirectional SSE (EventSource) endpoint (with emulation layer)")
	rootCmd.Flags().BoolP("http_stream.enabled", "", false, "enable bidirectional HTTP-streaming endpoint (with emulation layer)")
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
	)))

	if cmd != nil {
		bindPFlags := []string{
			"pid_file", "http_server.port", "http_server.address", "http_server.internal_port",
			"http_server.internal_address", "log.level", "log.file", "engine.type", "broker.enabled", "broker.type",
			"presence_manager.enabled", "presence_manager.type", "debug.enabled", "admin.enabled", "admin.external",
			"admin.insecure", "client.insecure", "http_api.insecure", "http_api.external", "prometheus.enabled",
			"health.enabled", "grpc_api.enabled", "grpc_api.port", "uni_grpc.enabled", "uni_grpc.port",
			"uni_websocket.enabled", "uni_sse.enabled", "uni_http_stream.enabled", "sse.enabled", "http_stream.enabled",
			"swagger.enabled",
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
		return Config{}, Meta{}, fmt.Errorf("error unmarshaling config: %w", err)
	}

	knownEnvVars := map[string]envconfig.VarInfo{}
	varInfo, err := envconfig.Process("CENTRIFUGO", conf)
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

	return *conf, meta, nil
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
			if _, ok := knownEnvVars[envKey]; !ok {
				unknownEnvs = append(unknownEnvs, envKey)
			}
		}
	}
	return unknownEnvs
}

var k8sEnvRegex = regexp.MustCompile(`^CENTRIFUGO(?:_[A-Z]+)?_(PORT|SERVICE_)`)

func isKubernetesEnvVar(envKey string) bool {
	return k8sEnvRegex.MatchString(envKey)
}

// DefaultConfig is a helper to be used in tests.
func DefaultConfig() Config {
	conf, _, err := GetConfig(nil, "")
	if err != nil {
		panic("error during getting default config: " + err.Error())
	}
	return conf
}
