package plugin

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/metrics"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/server"
)

type ClientFactory func(*node.Node, conns.Session) (conns.ClientConn, error)

var ClientFactories map[string]ClientFactory

// Allows to register custom ClientConn implementation.
func RegisterClient(name string, fn ClientFactory) {
	ClientFactories[name] = fn
}

type AdminFactory func(*node.Node, conns.Session) (conns.AdminConn, error)

var AdminFactories map[string]AdminFactory

// Allows to register custom AdminConn implementation.
func RegisterAdmin(name string, fn AdminFactory) {
	AdminFactories[name] = fn
}

type EngineFactory func(*node.Node, config.Getter) (engine.Engine, error)

var EngineFactories map[string]EngineFactory

// Allows to register custom Engine implementation.
func RegisterEngine(name string, fn EngineFactory) {
	EngineFactories[name] = fn
}

type Configurator func(config.Setter) error

var Configurators map[string]Configurator

// Allows to register custom configure function.
func RegisterConfigurator(name string, fn Configurator) {
	Configurators[name] = fn
}

var Metrics *metrics.Registry

type ServerFactory func(*node.Node, config.Getter) (server.Server, error)

var ServerFactories map[string]ServerFactory

// Allows to register custom Server implementation.
func RegisterServer(name string, fn ServerFactory) {
	ServerFactories[name] = fn
}

func init() {
	EngineFactories = map[string]EngineFactory{}
	ServerFactories = map[string]ServerFactory{}
	ClientFactories = map[string]ClientFactory{}
	AdminFactories = map[string]AdminFactory{}
	Configurators = map[string]Configurator{}
	Metrics = metrics.DefaultRegistry
}
