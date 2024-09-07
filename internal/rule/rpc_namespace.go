package rule

// RpcNamespace allows creating rules for different rpc.
type RpcNamespace struct {
	// Name is a unique rpc namespace name.
	Name string `mapstructure:"name" json:"name" envconfig:"name"`

	// Options for rpc namespace.
	RpcOptions `mapstructure:",squash"`
}

// RpcOptions can set a custom behaviour for rpc namespace.
type RpcOptions struct {
	// RpcProxyName which should be used for RPC namespace.
	RpcProxyName string `mapstructure:"rpc_proxy_name" json:"rpc_proxy_name,omitempty" envconfig:"rpc_proxy_name"`
}
