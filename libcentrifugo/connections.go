package libcentrifugo

// clientConnection is an interface abstracting all methods used
// by application to interact with client connection
type clientConn interface {
	// uid returns unique connection id
	uid() ConnID
	// project returns connection project key
	project() ProjectKey
	// user return user ID associated with connection
	user() UserID
	// channels returns a slice of channels connection subscribed to
	channels() []Channel
	// send allows to send message to connection client
	send(message string) error
	// unsubscribe allows to unsubscribe connection from channel
	unsubscribe(channel Channel) error
	// close closes client's connection
	close(reason string) error
}

// adminConnection is an interface abstracting all methods used
// by application to interact with admin connection
type adminConn interface {
	// uid returns unique admin connection id
	uid() string
	// send allows to send message to admin
	send(message string) error
}
