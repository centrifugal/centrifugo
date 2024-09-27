package configtypes

import (
	"encoding/json"
	"fmt"
)

type RPCNamespaces []RpcNamespace

// Decode to implement the envconfig.Decoder interface
func (d *RPCNamespaces) Decode(value string) error {
	// If the source is a string and the target is a slice, try to parse it as JSON.
	var items RPCNamespaces
	err := json.Unmarshal([]byte(value), &items)
	if err != nil {
		return fmt.Errorf("error parsing items from JSON: %v", err)
	}
	*d = items
	return nil
}

// RpcNamespace allows creating rules for different rpc.
type RpcNamespace struct {
	// Name is a unique rpc namespace name.
	Name string `mapstructure:"name" json:"name" envconfig:"name" yaml:"name" toml:"name"`

	// Options for rpc namespace.
	RpcOptions `mapstructure:",squash" yaml:",inline"`
}

// RpcOptions can set a custom behaviour for rpc namespace.
type RpcOptions struct {
	// RpcProxyName which should be used for RPC namespace.
	RpcProxyName string `mapstructure:"rpc_proxy_name" json:"rpc_proxy_name" envconfig:"rpc_proxy_name" yaml:"rpc_proxy_name" toml:"rpc_proxy_name"`
}
