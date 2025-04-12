package redisqueue

import (
	"context"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/redisshard"

	"github.com/redis/rueidis"
)

func CreateConsumerGroup(rds *redisshard.RedisShard, stream, groupName, id string) error {
	res := rds.RunOp(func(client rueidis.Client) rueidis.RedisResult {
		cmd := client.B().XgroupCreate().Key(stream).Group(groupName).Id(id).Mkstream().Build()
		return client.Do(context.Background(), cmd)
	})
	if res.Error() != nil && strings.Contains(res.Error().Error(), "BUSYGROUP") {
		// Group already exists.
		return nil
	}
	return res.Error()
}
