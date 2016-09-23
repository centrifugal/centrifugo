package plugin

// ConfigSetter allows to setup configuration options from pluggable components.
type ConfigSetter interface {
	SetDefault(key string, value interface{})
	BindEnv(key string)
	BindFlag(key string, flagName string)
	StringFlag(name, shorthand string, value string, usage string)
	BoolFlag(name, shorthand string, value bool, usage string)
	IntFlag(name, shorthand string, value int, usage string)
}

// ConfigGetter allows to get configuration options inside pluggable components.
type ConfigGetter interface {
	Get(string) interface{}
	GetString(string) string
	GetBool(string) bool
	GetInt(string) int
	IsSet(string) bool
}
