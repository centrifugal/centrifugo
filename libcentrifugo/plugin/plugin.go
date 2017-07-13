package plugin

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/metrics"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
)

// EngineFactory is a function that returns engine.Engine implementation.
type EngineFactory func(*node.Node, config.Getter) (engine.Engine, error)

// EngineFactories is a map where engine name matches EngineFactory.
var EngineFactories map[string]EngineFactory

// RegisterEngine allows to register custom Engine implementation.
func RegisterEngine(name string, fn EngineFactory) {
	EngineFactories[name] = fn
}

// Configurator is a function that can set default option values, flags, ENV vars.
type Configurator func(config.Setter) error

// Configurators is a map of Configurator functions.
var Configurators map[string]Configurator

// RegisterConfigurator allows to register custom configure function.
func RegisterConfigurator(name string, fn Configurator) {
	Configurators[name] = fn
}

// Metrics is pointer to registry to keep Centrifugo metrics.
var Metrics *metrics.Registry

func init() {
	EngineFactories = map[string]EngineFactory{}
	Configurators = map[string]Configurator{}
	Metrics = metrics.DefaultRegistry
}
