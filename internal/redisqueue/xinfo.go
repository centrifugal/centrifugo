package redisqueue

type XInfoGroup struct {
	Name            string
	LastDeliveredID string
	Consumers       int64
	Pending         int64
	EntriesRead     int64
	Lag             int64
}

type XInfoGroupsCmd struct {
	Err error
	Val []XInfoGroup
}
