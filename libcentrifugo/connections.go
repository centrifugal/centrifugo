package libcentrifugo

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

// clientConn is an interface abstracting all methods used
// by application to interact with client connection
type clientConn interface {
	// uid returns unique connection id.
	uid() proto.ConnID
	// user return user ID associated with connection.
	user() proto.UserID
	// channels returns a slice of channels connection subscribed to.
	channels() []proto.Channel
	// send allows to send message to connection client.
	send(message []byte) error
	// unsubscribe allows to unsubscribe connection from channel.
	unsubscribe(ch proto.Channel) error
	// close closes client's connection.
	close(reason string) error
}

// adminConn is an interface abstracting all methods used
// by application to interact with admin connection.
type adminConn interface {
	// uid returns unique admin connection id.
	uid() proto.ConnID
	// send allows to send message to admin connection.
	send(message []byte) error
}
