package router

type Config struct {
	Routes []struct {
		Name      string   `mapstructure:"name" json:"name"`
		Addresses []string `mapstructure:"addresses" json:"addresses"`
	} `mapstructure:"routes" json:"routes"`
}
