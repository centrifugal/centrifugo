package nowtime

import (
	"time"
)

type Getter func() time.Time

var _ Getter = Get

func Get() time.Time {
	return time.Now()
}
