package redisqueue

import (
	"fmt"

	"github.com/redis/rueidis"
)

type XAutoClaimCmd struct {
	err   error
	start string
	val   []rueidis.XRangeEntry
}

func NewXAutoClaimCmd(res rueidis.RedisResult) *XAutoClaimCmd {
	arr, err := res.ToArray()
	if err != nil {
		return &XAutoClaimCmd{err: err}
	}
	if len(arr) < 2 {
		return &XAutoClaimCmd{err: fmt.Errorf("got %d, wanted 2", len(arr))}
	}
	start, err := arr[0].ToString()
	if err != nil {
		return &XAutoClaimCmd{err: err}
	}
	ranges, err := arr[1].AsXRange()
	if err != nil {
		return &XAutoClaimCmd{err: err}
	}
	return &XAutoClaimCmd{val: ranges, start: start, err: err}
}

func (cmd *XAutoClaimCmd) Result() (messages []rueidis.XRangeEntry, start string, err error) {
	return cmd.val, cmd.start, cmd.err
}
