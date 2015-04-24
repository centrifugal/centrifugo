package main

// clientConnection is an interface abstracting all methods used
// by application to interact with client connection
type clientConnection interface {
	// getUid returns unique connection id
	getUid() string
	// getProject returns connection project key
	getProject() string
	// getUser return user ID associated with connection
	getUser() string
	// getChannels returns a slice of channels connection subscribed to
	getChannels() []string
	// send allows to send message to connection client
	send(message string) error
	// unsubscribe allows to unsubscribe connection from channel
	unsubscribe(channel string) error
	// close closes client's connection
	close(reason string) error
}

// adminConnection is an interface abstracting all methods used
// by application to interact with admin connection
type adminConnection interface {
	// getUid returns unique admin connection id
	getUid() string
	// send allows to send message to admin
	send(message string) error
}
