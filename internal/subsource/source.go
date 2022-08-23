package subsource

// Known sources of subscriptions in Centrifugo.
// Note: not using iota here intentionally.
const (
	Unknown           = 0
	ConnectionToken   = 1
	ConnectionCap     = 2
	ConnectProxy      = 3
	SubscriptionToken = 4
	UserLimited       = 5
	ClientAllowed     = 6
	ClientInsecure    = 7
	SubscribeProxy    = 8
	UniConnect        = 9
	UserPersonal      = 10
	ServerAPI         = 11
)
