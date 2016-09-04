package libcentrifugo

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/message"
)

// clientConn is an interface abstracting all methods used
// by application to interact with client connection
type clientConn interface {
	// uid returns unique connection id.
	uid() message.ConnID
	// user return user ID associated with connection.
	user() message.UserID
	// channels returns a slice of channels connection subscribed to.
	channels() []message.Channel
	// send allows to send message to connection client.
	send(message []byte) error
	// unsubscribe allows to unsubscribe connection from channel.
	unsubscribe(ch message.Channel) error
	// close closes client's connection.
	close(reason string) error
}

// adminConn is an interface abstracting all methods used
// by application to interact with admin connection.
type adminConn interface {
	// uid returns unique admin connection id.
	uid() message.ConnID
	// send allows to send message to admin connection.
	send(message []byte) error
}
