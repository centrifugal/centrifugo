package config

import (
	"github.com/FZambia/viper-lite"
	"github.com/spf13/pflag"
)

type viperConfigSetter struct {
	viper   *viper.Viper
	flagSet *pflag.FlagSet
}

// NewViperConfigSetter is a wrapper over viper to return ConfigSetter interface from it.
func NewViperConfigSetter(v *viper.Viper, fs *pflag.FlagSet) Setter {
	return &viperConfigSetter{
		viper:   v,
		flagSet: fs,
	}
}

func (s *viperConfigSetter) StringFlag(name, shorthand string, value string, usage string) {
	var p string
	s.flagSet.StringVarP(&p, name, shorthand, value, usage)
}

func (s *viperConfigSetter) BoolFlag(name, shorthand string, value bool, usage string) {
	var p bool
	s.flagSet.BoolVarP(&p, name, shorthand, value, usage)
}

func (s *viperConfigSetter) IntFlag(name, shorthand string, value int, usage string) {
	var p int
	s.flagSet.IntVarP(&p, name, shorthand, value, usage)
}

func (s *viperConfigSetter) SetDefault(key string, value interface{}) {
	s.viper.SetDefault(key, value)
}

func (s *viperConfigSetter) BindEnv(key string) {
	s.viper.BindEnv(key)
}

func (s *viperConfigSetter) BindFlag(key string, flagName string) {
	s.viper.BindPFlag(key, s.flagSet.Lookup(flagName))
}

type viperConfigGetter struct {
	viper *viper.Viper
}

// NewViperConfigGetter is a wrapper over viper to return ConfigGetter interface from it.
func NewViperConfigGetter(v *viper.Viper) Getter {
	return &viperConfigGetter{
		viper: v,
	}
}

func (g *viperConfigGetter) Get(key string) interface{} {
	return g.viper.Get(key)
}

func (g *viperConfigGetter) GetString(key string) string {
	return g.viper.GetString(key)
}

func (g *viperConfigGetter) GetInt(key string) int {
	return g.viper.GetInt(key)
}

func (g *viperConfigGetter) GetBool(key string) bool {
	return g.viper.GetBool(key)
}

func (g *viperConfigGetter) IsSet(key string) bool {
	return g.viper.IsSet(key)
}

func (g *viperConfigGetter) UnmarshalKey(key string, target interface{}) error {
	return g.viper.UnmarshalKey(key, target)
}
