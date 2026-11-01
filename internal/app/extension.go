package app

import (
	"fmt"
	"io"
	"net/http"

	"github.com/centrifugal/centrifugo/v6/internal/api"
	"github.com/centrifugal/centrifugo/v6/internal/client"
	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/centrifugal/centrifuge"
)

// Extension is a component added to the App through Options.Extensions.
// The App calls the optional interfaces below that an extension implements,
// once per App, at fixed points of New and Start, in the order extensions
// are listed in Options.
type Extension interface {
	Name() string
}

// LogWriterProvider gives writers receiving every log line. Called before
// logging is set up; the returned function (may be nil) is called on Close.
type LogWriterProvider interface {
	LogWriters(cfg config.Config) ([]io.Writer, func(), error)
}

// Setupper is called once the node, client handler and executors exist,
// before the client handler registers its node callbacks.
type Setupper interface {
	Setup(c *Components) error
}

// MuxExtender adds routes to the HTTP servers.
type MuxExtender interface {
	ExtendMux(m *MuxContext) error
}

// ConfigObserver is told about config changes the App rejected or applied.
type ConfigObserver interface {
	// ConfigChangeRejected is called when a config file change was rejected.
	ConfigChangeRejected(err error)
	// ConfigReloaded is called with the outcome of a SIGHUP reload.
	ConfigReloaded(err error)
}

// Components are the parts of a running App given to Setupper extensions.
type Components struct {
	ConfigFile        string
	Container         *config.Container
	Node              *centrifuge.Node
	ClientHandler     *client.Handler
	HTTPExecutor      *api.Executor
	GRPCExecutor      *api.Executor
	ConsumingExecutor *api.Executor

	app *App
}

// Lifecycle returns the App's Lifecycle.
func (c *Components) Lifecycle() *Lifecycle {
	return c.app.lifecycle
}

// Port selects the HTTP server a Route is added to.
type Port int

const (
	// PortExternal is the server of http_server.address and http_server.port.
	PortExternal Port = iota
	// PortInternal is the server of the internal address and port (the
	// external one when they are not set).
	PortInternal
)

// Chain selects the middlewares a Route's handler is wrapped with.
type Chain int

const (
	// ChainBasic wraps the handler with request logging and metrics, as for
	// built-in internal endpoints.
	ChainBasic Chain = iota
	// ChainNone registers the handler as is.
	ChainNone
)

// Route is an HTTP endpoint added by a MuxExtender.
type Route struct {
	// Name is shown in the "serving ... endpoints" log line.
	Name    string
	Port    Port
	Pattern string
	Handler http.Handler
	Chain   Chain
	// IfAbsent registers the route only when no other route registered
	// exactly this pattern. Such routes are added last, and only to servers
	// which start anyway.
	IfAbsent bool
}

// MuxContext is given to MuxExtender extensions. AddRoute must be called
// from ExtendMux: a later call panics.
type MuxContext struct {
	Config    config.Config
	Container *config.Container
	Node      *centrifuge.Node
	Lifecycle *Lifecycle

	routes []Route
	// sealed is set once every MuxExtender returned: Start then reads the
	// added routes.
	sealed bool
}

// errSealed is the error of a MuxContext method called after the App read
// what it registers.
func errSealed(method, when string) error {
	return fmt.Errorf("%s called after %s: it must be called before", method, when)
}

// checkSealed panics when Start already read what method registers.
func (m *MuxContext) checkSealed(method string) {
	if m.sealed {
		panic(errSealed("MuxContext."+method, "ExtendMux").Error())
	}
}

// AddRoute adds an HTTP route. A route with the pattern of a built-in or
// another route fails Start, unless it is IfAbsent.
func (m *MuxContext) AddRoute(r Route) {
	m.checkSealed("AddRoute")
	m.routes = append(m.routes, r)
}
