package wt

// Config for Handler.
type Config struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/webtransport"`
}
