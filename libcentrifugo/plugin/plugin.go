package plugin

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/metrics"
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

var Metrics metrics.MetricsRegistry

var Servers map[string]server.Server

type ServerFactory func(server.Node, ConfigGetter) (server.Server, error)

var ServerFactories map[string]ServerFactory

func RegisterServer(name string, fn ServerFactory) {
	ServerFactories[name] = fn
}

func init() {
	EngineFactories = map[string]EngineFactory{}
	ServerFactories = map[string]ServerFactory{}
	Configurators = map[string]Configurator{}
	Metrics = metrics.Metrics
}
