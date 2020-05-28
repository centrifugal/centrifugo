package centrifuge

import "github.com/centrifugal/protocol"

// Define some type aliases to protocol types.
//
// Some reasoning to use aliases here is to prevent user's code to directly import
// centrifugal/protocol package.
//
// Theoretically we could provide wrappers to protocol types for centrifuge public API
// but copying protocol types to wrapper types introduces additional overhead so we
// decided to be low-level here for now. This is a subject to think about for future
// releases though since users that need maximum performance can use Engine methods
// directly.
//
// TODO v1: decide whether to keep performance or safer public API.
type (
	// Publication contains Data sent to channel subscribers.
	// In channels with recover option on it also has incremental Offset.
	// If Publication sent from client side it can also have Info (otherwise nil).
	Publication = protocol.Publication
	// ClientInfo contains information about client connection.
	ClientInfo = protocol.ClientInfo
	// Raw represents raw untouched bytes.
	Raw = protocol.Raw
)
