package redisqueue

import (
	"fmt"
	"time"

	"github.com/redis/rueidis"
)

type XPendingExt struct {
	ID         string
	Consumer   string
	Idle       time.Duration
	RetryCount int64
}

type XPendingExtCmd struct {
	Err error
	Val []XPendingExt
}

func NewXPendingExtCmd(res rueidis.RedisResult) *XPendingExtCmd {
	arrs, err := res.ToArray()
	if err != nil {
		return &XPendingExtCmd{Err: err}
	}
	val := make([]XPendingExt, 0, len(arrs))
	for _, v := range arrs {
		arr, err := v.ToArray()
		if err != nil {
			return &XPendingExtCmd{Err: err}
		}
		if len(arr) < 4 {
			return &XPendingExtCmd{Err: fmt.Errorf("got %d, wanted 4", len(arr))}
		}
		id, err := arr[0].ToString()
		if err != nil {
			return &XPendingExtCmd{Err: err}
		}
		consumer, err := arr[1].ToString()
		if err != nil {
			return &XPendingExtCmd{Err: err}
		}
		idle, err := arr[2].AsInt64()
		if err != nil {
			return &XPendingExtCmd{Err: err}
		}
		retryCount, err := arr[3].AsInt64()
		if err != nil {
			return &XPendingExtCmd{Err: err}
		}
		val = append(val, XPendingExt{
			ID:         id,
			Consumer:   consumer,
			Idle:       time.Duration(idle) * time.Millisecond,
			RetryCount: retryCount,
		})
	}
	return &XPendingExtCmd{Val: val, Err: err}
}
