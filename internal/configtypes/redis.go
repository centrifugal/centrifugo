package configtypes

type Redis struct {
	Address            []string  `mapstructure:"address" json:"address" envconfig:"address" default:"redis://127.0.0.1:6379" yaml:"address" toml:"address"`
	Prefix             string    `mapstructure:"prefix" json:"prefix" envconfig:"prefix" default:"centrifugo" yaml:"prefix" toml:"prefix"`
	ConnectTimeout     Duration  `mapstructure:"connect_timeout" json:"connect_timeout" envconfig:"connect_timeout" default:"1s" yaml:"connect_timeout" toml:"connect_timeout"`
	IOTimeout          Duration  `mapstructure:"io_timeout" json:"io_timeout" envconfig:"io_timeout" default:"4s" yaml:"io_timeout" toml:"io_timeout"`
	DB                 int       `mapstructure:"db" json:"db" envconfig:"db" default:"0" yaml:"db" toml:"db"`
	User               string    `mapstructure:"user" json:"user" envconfig:"user" yaml:"user" toml:"user"`
	Password           string    `mapstructure:"password" json:"password" envconfig:"password" yaml:"password" toml:"password"`
	ClientName         string    `mapstructure:"client_name" json:"client_name" envconfig:"client_name" yaml:"client_name" toml:"client_name"`
	ForceResp2         bool      `mapstructure:"force_resp2" json:"force_resp2" envconfig:"force_resp2" yaml:"force_resp2" toml:"force_resp2"`
	ClusterAddress     []string  `mapstructure:"cluster_address" json:"cluster_address" envconfig:"cluster_address" yaml:"cluster_address" toml:"cluster_address"`
	TLS                TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls"`
	SentinelAddress    []string  `mapstructure:"sentinel_address" json:"sentinel_address" envconfig:"sentinel_address" yaml:"sentinel_address" toml:"sentinel_address"`
	SentinelUser       string    `mapstructure:"sentinel_user" json:"sentinel_user" envconfig:"sentinel_user" yaml:"sentinel_user" toml:"sentinel_user"`
	SentinelPassword   string    `mapstructure:"sentinel_password" json:"sentinel_password" envconfig:"sentinel_password" yaml:"sentinel_password" toml:"sentinel_password"`
	SentinelMasterName string    `mapstructure:"sentinel_master_name" json:"sentinel_master_name" envconfig:"sentinel_master_name" yaml:"sentinel_master_name" toml:"sentinel_master_name"`
	SentinelClientName string    `mapstructure:"sentinel_client_name" json:"sentinel_client_name" envconfig:"sentinel_client_name" yaml:"sentinel_client_name" toml:"sentinel_client_name"`
	SentinelTLS        TLSConfig `mapstructure:"sentinel_tls" json:"sentinel_tls" envconfig:"sentinel_tls" yaml:"sentinel_tls" toml:"sentinel_tls"`
}
