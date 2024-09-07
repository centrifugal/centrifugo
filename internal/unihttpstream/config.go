package unihttpstream

type Config struct {
	Enabled            bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/uni_http_stream"`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536"`
}
