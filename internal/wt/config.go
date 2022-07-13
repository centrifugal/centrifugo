package wt

import "time"

// Config for Handler.
type Config struct {
	// AppLevelPingInterval tells how often to issue application-level server-to-client pings.
	// AppLevelPingInterval is only used for clients with ProtocolVersion2.
	// AppLevelPingInterval is EXPERIMENTAL and is a subject to change.
	// For zero value 25 secs will be used. To disable sending app-level pings use -1.
	AppLevelPingInterval time.Duration
	// AppLevelPongTimeout sets time for application-level pong check after issuing
	// ping. AppLevelPongTimeout must be less than AppLevelPingInterval.
	// AppLevelPongTimeout is only used for clients with ProtocolVersion2.
	// AppLevelPongTimeout is EXPERIMENTAL and is a subject to change.
	// For zero value AppLevelPingInterval / 3 will be used. To disable pong checks use -1.
	AppLevelPongTimeout time.Duration
}
