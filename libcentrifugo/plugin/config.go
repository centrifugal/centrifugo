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
	Get(key string) interface{}
	GetString(key string) string
	GetBool(key string) bool
	GetInt(key string) int
	IsSet(key string) bool
	UnmarshalKey(key string, target interface{}) error
}
