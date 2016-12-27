package server

// Server is an interface describing custom server that could be registered in Centrifugo.
// At moment we have only httpserver (located nearby) that implements it.
type Server interface {
	// Run runs the server once node is ready.
	Run() error

	// Shutdown shuts down the server.
	Shutdown() error
}
