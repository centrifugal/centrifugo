package server

type Server interface {
	// Run runs the server once node is ready.
	Run() error

	// Reload called when Centrifugo configuration has been reloaded.
	// It's pretty hard to reload everything but can be used at least for critical options.
	// New config getter can be asked from Node.
	Reload() error

	// Shutdown shuts down the server.
	Shutdown() error
}
