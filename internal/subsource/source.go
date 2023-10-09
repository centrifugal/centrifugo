package subsource

// Known sources of subscriptions in Centrifugo.
// Note: not using iota here intentionally.
const (
	Unknown           = 0
	ConnectionToken   = 1
	ConnectProxy      = 2
	SubscriptionToken = 3
	SubscribeProxy    = 4
	UserLimited       = 5
	ClientAllowed     = 6
	ClientInsecure    = 7
	UniConnect        = 8
	UserPersonal      = 9
	ServerAPI         = 10
	ConnectionCap     = 11
	Expression        = 12
	StreamProxy       = 13
)
