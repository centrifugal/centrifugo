package plugin

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/server"
)

type EngineFactory func(server.Node, ConfigGetter) (engine.Engine, error)

var EngineFactories map[string]EngineFactory

func RegisterEngine(name string, fn EngineFactory) {
	EngineFactories[name] = fn
}

type Configurator func(ConfigSetter) error

var Configurators map[string]Configurator

func RegisterConfigurator(name string, fn Configurator) {
	Configurators[name] = fn
}

func init() {
	EngineFactories = map[string]EngineFactory{}
	Configurators = map[string]Configurator{}
}
