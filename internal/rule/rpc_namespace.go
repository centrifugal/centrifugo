package rule

// RpcNamespace allows to create rules for different rpc.
type RpcNamespace struct {
	// Name is a unique namespace name.
	Name string `mapstructure:"name" json:"name"`

	// Options for rpc namespace.
	RpcOptions `mapstructure:",squash"`
}

// RpcOptions ...
type RpcOptions struct {
	// RpcProxyName which should be used for RPC namespace.
	RpcProxyName string `mapstructure:"rpc_proxy_name" json:"rpc_proxy_name,omitempty"`
}
