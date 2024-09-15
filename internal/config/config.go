// Package config contains Centrifugo Config and the code to load it.
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/config/envconfig"
	"github.com/centrifugal/centrifugo/v5/internal/configtypes"

	"github.com/hashicorp/go-envparse"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Config struct {
	// Address to bind HTTP server to.
	Address string `mapstructure:"address" json:"address" envconfig:"address"`
	// Port to bind HTTP server to.
	Port int `mapstructure:"port" json:"port" envconfig:"port" default:"8000"`
	// InternalAddress to bind internal HTTP server to. Internal server is used to serve endpoints
	// which are normally should not be exposed to the outside world.
	InternalAddress string `mapstructure:"internal_address" json:"internal_address" envconfig:"internal_address"`
	// InternalPort to bind internal HTTP server to.
	InternalPort string `mapstructure:"internal_port" json:"internal_port" envconfig:"internal_port"`
	// PidFile is a path to write PID file with server's PID.
	PidFile string `mapstructure:"pid_file" json:"pid_file" envconfig:"pid_file"`
	// LogLevel is a log level for Centrifugo logger. Supported values: none, trace, debug, info, warn, error.
	LogLevel string `mapstructure:"log_level" json:"log_level" envconfig:"log_level" default:"info"`
	// LogFile is a path to log file. If not set logs go to stdout.
	LogFile string `mapstructure:"log_file" json:"log_file" envconfig:"log_file"`

	// Engine to use: memory or redis. By default, memory engine is used. Memory engine is superfast,
	// but it's not distributed and all data stored in memory (thus lost after node restart). Redis engine
	// provides seamless horizontal scalability, fault-tolerance, and persistence over Centrifugo restarts.
	// See also Broker option to run Centrifugo with Nats (only implements at most once PUB/SUB semantics).
	Engine string `mapstructure:"engine" json:"engine" envconfig:"engine" default:"memory"`
	// Broker to use: the only option is nats.
	Broker string `mapstructure:"broker" json:"broker" envconfig:"broker"`

	// TLS configuration for HTTP server.
	TLS configtypes.TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls"`
	// TLSAutocert for automatic TLS certificates from ACME provider (ex. Let's Encrypt).
	TLSAutocert configtypes.TLSAutocert `mapstructure:"tls_autocert" json:"tls_autocert" envconfig:"tls_autocert"`
	// TLSExternal enables TLS only for external HTTP endpoints.
	TLSExternal bool `mapstructure:"tls_external" json:"tls_external" envconfig:"tls_external"`

	// HTTP3 enables HTTP/3 support. EXPERIMENTAL.
	HTTP3 bool `mapstructure:"http3" json:"http3" envconfig:"http3"`

	// WebSocket configuration. This transport is enabled by default.
	WebSocket configtypes.WebSocket `mapstructure:"websocket" json:"websocket" envconfig:"websocket"`
	// SSE is a configuration for Server-Sent Events based bidirectional emulation transport.
	SSE configtypes.SSE `mapstructure:"sse" json:"sse" envconfig:"sse"`
	// HTTPStream is a configuration for HTTP streaming based bidirectional emulation transport.
	HTTPStream configtypes.HTTPStream `mapstructure:"http_stream" json:"http_stream" envconfig:"http_stream"`
	// WebTransport is a configuration for WebTransport transport. EXPERIMENTAL.
	WebTransport configtypes.WebTransport `mapstructure:"webtransport" json:"webtransport" envconfig:"webtransport"`
	// UniSSE is a configuration for unidirectional Server-Sent Events transport.
	UniSSE configtypes.UniSSE `mapstructure:"uni_sse" json:"uni_sse" envconfig:"uni_sse"`
	// UniHTTPStream is a configuration for unidirectional HTTP streaming transport.
	UniHTTPStream configtypes.UniHTTPStream `mapstructure:"uni_http_stream" json:"uni_http_stream" envconfig:"uni_http_stream"`
	// UniWS is a configuration for unidirectional WebSocket transport.
	UniWS configtypes.UniWebSocket `mapstructure:"uni_websocket" json:"uni_websocket" envconfig:"uni_websocket"`
	// UniGRPC is a configuration for unidirectional gRPC transport.
	UniGRPC configtypes.UniGRPC `mapstructure:"uni_grpc" json:"uni_grpc" envconfig:"uni_grpc"`
	// Emulation endpoint is enabled automatically when at least one bidirectional emulation transport
	// is configured (SSE or HTTP Stream).
	Emulation configtypes.Emulation `mapstructure:"emulation" json:"emulation" envconfig:"emulation"`
	// Admin web UI configuration.
	Admin configtypes.Admin `mapstructure:"admin" json:"admin" envconfig:"admin"`
	// Prometheus metrics configuration.
	Prometheus configtypes.Prometheus `mapstructure:"prometheus" json:"prometheus" envconfig:"prometheus"`
	// Health check endpoint configuration.
	Health configtypes.Health `mapstructure:"health" json:"health" envconfig:"health"`
	// Swagger documentation (for server HTTP API) configuration.
	Swagger configtypes.Swagger `mapstructure:"swagger" json:"swagger" envconfig:"swagger"`
	// Debug helps to enable Go profiling endpoints.
	Debug configtypes.Debug `mapstructure:"debug" json:"debug" envconfig:"debug"`

	// Consumers is a configuration for message queue consumers. For example, Centrifugo can consume
	// messages from PostgreSQL transactional outbox table, or from Kafka topics.
	Consumers configtypes.Consumers `mapstructure:"consumers" json:"consumers" envconfig:"consumers"`

	// GlobalHistoryTTL is a time how long to keep history meta information. This is a global option for all channels,
	// but it can be overridden in channel namespace.
	GlobalHistoryMetaTTL time.Duration `mapstructure:"global_history_meta_ttl" json:"global_history_meta_ttl" envconfig:"global_history_meta_ttl" default:"720h"`
	// GlobalPresenceTTL is a time how long to keep presence information if not updated. This is a global option for all channels.
	GlobalPresenceTTL time.Duration `mapstructure:"global_presence_ttl" json:"global_presence_ttl" envconfig:"global_presence_ttl" default:"60s"`

	// Client contains real-time client connection related configuration.
	Client configtypes.Client `mapstructure:"client" json:"client" envconfig:"client"`
	// Channel contains real-time channel related configuration.
	Channel configtypes.Channel `mapstructure:"channel" json:"channel" envconfig:"channel"`
	// RPC is a configuration for client RPC calls.
	RPC configtypes.RPC `mapstructure:"rpc" json:"rpc" envconfig:"rpc"`

	// UserSubscribeToPersonal is a configuration for a feature to automatically subscribe user to a personal channel
	// using server-side subscription.
	UserSubscribeToPersonal configtypes.UserSubscribeToPersonal `mapstructure:"user_subscribe_to_personal" json:"user_subscribe_to_personal" envconfig:"user_subscribe_to_personal"`

	// HttpAPI is a configuration for HTTP server API. It's enabled by default.
	HttpAPI configtypes.HttpAPI `mapstructure:"http_api" json:"http_api" envconfig:"http_api"`
	// GrpcAPI is a configuration for gRPC server API. It's disabled by default.
	GrpcAPI configtypes.GrpcAPI `mapstructure:"grpc_api" json:"grpc_api" envconfig:"grpc_api"`
	// Redis is a configuration for Redis engine.
	Redis configtypes.RedisEngine `mapstructure:"redis" json:"redis" envconfig:"redis"`
	// Nats is a configuration for NATS broker.
	Nats configtypes.NatsBroker `mapstructure:"nats" json:"nats" envconfig:"nats"`

	// Proxy is a configuration for global events proxy. See also GranularProxyMode.
	Proxy configtypes.GlobalProxy `mapstructure:"proxy" json:"proxy" envconfig:"proxy"`
	// GranularProxyMode enables granular proxy mode. Using this mode, it's possible to configure separate
	// proxies for different types of events. And separate proxies for different channel namespaces.
	GranularProxyMode bool `mapstructure:"granular_proxy_mode" json:"granular_proxy_mode" envconfig:"granular_proxy_mode"`
	// Proxies is a configuration for granular events proxies. See also GranularProxyMode.
	Proxies configtypes.Proxies `mapstructure:"proxies" json:"proxies" envconfig:"proxies"`
	// ConnectProxyName is a name of proxy to use for connect events when GranularProxyMode is used.
	ConnectProxyName string `mapstructure:"connect_proxy_name" json:"connect_proxy_name" envconfig:"connect_proxy_name"`
	// RefreshProxyName is a name of proxy to use for refresh events when GranularProxyMode is used.
	RefreshProxyName string `mapstructure:"refresh_proxy_name" json:"refresh_proxy_name" envconfig:"refresh_proxy_name"`

	// NodeInfoMetricsAggregateInterval is a time interval to aggregate node info metrics.
	NodeInfoMetricsAggregateInterval time.Duration `mapstructure:"node_info_metrics_aggregate_interval" json:"node_info_metrics_aggregate_interval" envconfig:"node_info_metrics_aggregate_interval" default:"60s"`

	// OpenTelemetry is a configuration for OpenTelemetry tracing.
	OpenTelemetry configtypes.OpenTelemetry `mapstructure:"opentelemetry" json:"opentelemetry" envconfig:"opentelemetry"`
	// Graphite is a configuration for export metrics to Graphite.
	Graphite configtypes.Graphite `mapstructure:"graphite" json:"graphite" envconfig:"graphite"`
	// UsageStats is a configuration for usage stats sending.
	UsageStats configtypes.UsageStats `mapstructure:"usage_stats" json:"usage_stats" envconfig:"usage_stats"`
	// Shutdown is a configuration for graceful shutdown.
	Shutdown configtypes.Shutdown `mapstructure:"shutdown" json:"shutdown" envconfig:"shutdown"`
	// Name is a human-readable name of Centrifugo node. This must be unique for each running node in a cluster.
	// By default, Centrifugo uses hostname and port. Name is shown in admin web interface. For communication
	// between nodes in a cluster, Centrifugo uses another identifier – unique ID generated on node start.
	Name string `mapstructure:"name" json:"name" envconfig:"name"`

	// EnableUnreleasedFeatures enables unreleased features. These features are not stable and may be removed even
	// in minor release update. Evaluate and share feedback if you find some feature useful and want it to be stabilized.
	EnableUnreleasedFeatures bool `mapstructure:"enable_unreleased_features" json:"enable_unreleased_features" envconfig:"enable_unreleased_features"`
}

type Meta struct {
	FileNotFound bool
	UnknownKeys  []string
	UnknownEnvs  []string
}

func DefineFlags(rootCmd *cobra.Command) {
	rootCmd.Flags().StringP("address", "a", "", "interface address to listen on")
	rootCmd.Flags().StringP("port", "p", "8000", "port to bind HTTP server to")
	rootCmd.Flags().StringP("internal_address", "", "", "custom interface address to listen on for internal endpoints")
	rootCmd.Flags().StringP("internal_port", "", "", "custom port for internal endpoints")
	rootCmd.Flags().StringP("engine", "e", "memory", "engine to use: memory or redis")
	rootCmd.Flags().StringP("broker", "", "", "custom broker to use: ex. nats")
	rootCmd.Flags().StringP("log_level", "", "info", "set the log level: trace, debug, info, error, fatal or none")
	rootCmd.Flags().StringP("log_file", "", "", "optional log file - if not specified logs go to STDOUT")
	rootCmd.Flags().StringP("pid_file", "", "", "optional path to create PID file")
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
	v := viper.NewWithOptions()

	if cmd != nil {
		bindPFlags := []string{
			"port", "address", "internal_port", "internal_address", "log_level", "log_file", "pid_file",
			"engine", "broker", "debug.enabled", "admin.enabled", "admin.external", "admin.insecure",
			"client.insecure", "http_api.insecure", "http_api.external", "prometheus.enabled", "health.enabled",
			"grpc_api.enabled", "grpc_api.port", "uni_grpc.enabled", "uni_grpc.port", "uni_websocket.enabled",
			"uni_sse.enabled", "uni_http_stream.enabled", "sse.enabled", "http_stream.enabled", "swagger.enabled",
		}
		for _, flag := range bindPFlags {
			_ = v.BindPFlag(flag, cmd.Flags().Lookup(flag))
		}
	}

	v.SetConfigFile(configFile)

	meta := Meta{}

	err := v.ReadInConfig()
	if err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			meta.FileNotFound = true
		} else {
			return Config{}, Meta{}, fmt.Errorf("error reading config file: %w", err)
		}
	}

	conf := &Config{}

	err = v.Unmarshal(conf)
	if err != nil {
		return Config{}, Meta{}, fmt.Errorf("error unmarshaling config: %w", err)
	}

	knownEnvVars := map[string]struct{}{}
	varInfo, err := envconfig.Process("CENTRIFUGO", conf)
	if err != nil {
		return Config{}, Meta{}, fmt.Errorf("error processing env: %w", err)
	}
	extendKnownEnvVars(knownEnvVars, varInfo)

	for i, item := range conf.Channel.Namespaces {
		varInfo, err = envconfig.Process("CENTRIFUGO_CHANNEL_NAMESPACES_"+item.Name, &item)
		if err != nil {
			return Config{}, Meta{}, fmt.Errorf("error processing env namespaces: %w", err)
		}
		conf.Channel.Namespaces[i] = item
		extendKnownEnvVars(knownEnvVars, varInfo)
	}

	for i, item := range conf.RPC.Namespaces {
		varInfo, err = envconfig.Process("CENTRIFUGO_RPC_NAMESPACES_"+item.Name, &item)
		if err != nil {
			return Config{}, Meta{}, fmt.Errorf("error processing env rpc namespaces: %w", err)
		}
		conf.RPC.Namespaces[i] = item
		extendKnownEnvVars(knownEnvVars, varInfo)
	}

	for i, item := range conf.Proxies {
		varInfo, err = envconfig.Process("CENTRIFUGO_PROXIES_"+item.Name, &item)
		if err != nil {
			return Config{}, Meta{}, fmt.Errorf("error processing env proxies: %w", err)
		}
		conf.Proxies[i] = item
		extendKnownEnvVars(knownEnvVars, varInfo)
	}

	for i, item := range conf.Consumers {
		varInfo, err = envconfig.Process("CENTRIFUGO_CONSUMERS_"+item.Name, &item)
		if err != nil {
			return Config{}, Meta{}, fmt.Errorf("error processing env consumers: %w", err)
		}
		conf.Consumers[i] = item
		extendKnownEnvVars(knownEnvVars, varInfo)
	}

	for i, header := range conf.Proxy.HttpHeaders {
		conf.Proxy.HttpHeaders[i] = strings.ToLower(header)
	}

	meta.UnknownKeys = findUnknownKeys(v.AllSettings(), conf, "")
	meta.UnknownEnvs = checkEnvironmentVars(knownEnvVars)

	return *conf, meta, nil
}

func extendKnownEnvVars(knownEnvVars map[string]struct{}, varInfo []envconfig.VarInfo) {
	for _, info := range varInfo {
		knownEnvVars[info.Key] = struct{}{}
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

	// Build a set of valid keys from the struct's mapstructure tags, including embedded structs
	validKeys := make(map[string]reflect.StructField)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("mapstructure")
		if tag != "" && tag != ",squash" {
			validKeys[tag] = field
		} else if field.Anonymous && strings.Contains(tag, "squash") {
			embeddedType := field.Type
			if embeddedType.Kind() == reflect.Ptr {
				embeddedType = embeddedType.Elem()
			}
			if embeddedType.Kind() == reflect.Struct {
				for j := 0; j < embeddedType.NumField(); j++ {
					embeddedField := embeddedType.Field(j)
					embeddedTag := embeddedField.Tag.Get("mapstructure")
					if embeddedTag != "" {
						validKeys[embeddedTag] = embeddedField
					}
				}
			}
		}
	}

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

func checkEnvironmentVars(knownEnvVars map[string]struct{}) []string {
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

var k8sPrefixes = []string{
	"CENTRIFUGO_PORT_",
	"CENTRIFUGO_SERVICE_",
}

func isKubernetesEnvVar(envKey string) bool {
	for _, k8sPrefix := range k8sPrefixes {
		if strings.HasPrefix(envKey, k8sPrefix) {
			return true
		}
	}
	return false
}

// DefaultConfig is a helper to be used in tests.
func DefaultConfig() Config {
	conf, _, err := GetConfig(nil, "-")
	if err != nil {
		panic("error during getting default config: " + err.Error())
	}
	return conf
}
