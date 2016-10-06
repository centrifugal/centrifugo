package server

type Server interface {
	// Run runs the server once node is ready.
	Run() error

	// Shutdown shuts down the server.
	Shutdown() error
}
