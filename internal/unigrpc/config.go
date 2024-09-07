package unigrpc

import "github.com/centrifugal/centrifugo/v5/internal/configtypes"

type Config struct {
	Enabled               bool                  `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	Address               string                `mapstructure:"address" json:"address" envconfig:"address"`
	Port                  int                   `mapstructure:"port" json:"port" envconfig:"port" default:"11000"`
	MaxReceiveMessageSize int                   `mapstructure:"max_receive_message_size" json:"max_receive_message_size" envconfig:"max_receive_message_size"`
	TLS                   configtypes.TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls"`
}
