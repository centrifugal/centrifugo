package main

// clientConnection is an interface abstracting all methods used
// by application to interact with client connection
type clientConnection interface {
	getUid() string
	getProject() string
	getUser() string
	send(message string) error
}

// adminConnection is an interface abstracting all methods used
// by application to interact with admin connection
type adminConnection interface {
	getUid() string
	send(message string) error
}
