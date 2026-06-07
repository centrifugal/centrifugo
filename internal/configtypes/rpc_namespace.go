package configtypes

type RPCNamespaces []RpcNamespace

// Decode to implement the envconfig.Decoder interface
func (d *RPCNamespaces) Decode(value string) error {
	return decodeToNamedSlice(value, d)
}

// RpcNamespace allows creating rules for different rpc.
type RpcNamespace struct {
	// Name is a unique rpc namespace name.
	Name string `mapstructure:"name" json:"name" envconfig:"name" yaml:"name" toml:"name" expose:"full" doc:"Unique RPC namespace name. RPC methods prefixed with <<name:>> use this namespace's options."`

	// Options for rpc namespace.
	RpcOptions `mapstructure:",squash" yaml:",inline"`
}

// RpcOptions can set a custom behaviour for rpc namespace.
type RpcOptions struct {
	// ProxyEnabled allows to enable using RPC proxy for this namespace.
	ProxyEnabled bool `mapstructure:"proxy_enabled" json:"proxy_enabled" envconfig:"proxy_enabled" yaml:"proxy_enabled" toml:"proxy_enabled" doc:"Proxy RPC calls in this namespace to your backend. Requires a configured RPC proxy."`
	// ProxyName which should be used for RPC namespace.
	ProxyName string `mapstructure:"proxy_name" default:"default" json:"proxy_name" envconfig:"proxy_name" yaml:"proxy_name" toml:"proxy_name" expose:"full" doc:"Name of the configured proxy to use for RPC calls in this namespace. Defaults to <<default>>."`
}
