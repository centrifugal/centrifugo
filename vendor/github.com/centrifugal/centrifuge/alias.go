package centrifuge

import (
	"github.com/centrifugal/centrifuge/internal/proto"
)

// Error represents client reply error.
type Error = proto.Error

// Raw represents raw bytes.
type Raw = proto.Raw

// Publication allows to deliver custom payload to all channel subscribers.
type Publication = proto.Publication

// Join sent to channel after someone subscribed.
type Join = proto.Join

// Leave sent to channel after someone unsubscribed.
type Leave = proto.Leave

// ClientInfo is short information about client connection.
type ClientInfo = proto.ClientInfo

// Encoding represents client connection transport encoding format.
type Encoding = proto.Encoding
