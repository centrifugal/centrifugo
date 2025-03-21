//go:build integration

package redisqueue

import (
	"context"
	"testing"

	"github.com/redis/rueidis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProducer(t *testing.T) {
	t.Run("creates a new producer", func(tt *testing.T) {
		e := testRedisShard(t)
		defer e.Close()
		p, err := NewProducer(e, ProducerOptions{
			Stream: tt.Name(),
		})
		require.NoError(tt, err)
		assert.NotNil(tt, p)
	})
}

func TestEnqueue(t *testing.T) {
	t.Run("puts the message in the stream", func(tt *testing.T) {
		e := testRedisShard(t)
		defer e.Close()
		p, err := NewProducer(e, ProducerOptions{
			Stream: tt.Name(),
		})
		require.NoError(t, err)

		msg := &Message{
			Values: map[string]string{"test": "value"},
		}
		err = p.Enqueue(msg)
		require.NoError(tt, err)

		res := p.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
			cmd := client.B().Xrange().Key(tt.Name()).Start(msg.ID).End(msg.ID).Build()
			return client.Do(context.Background(), cmd)
		})
		require.NoError(tt, res.Error())

		xRange, err := res.AsXRange()
		require.NoError(tt, err)

		assert.Equal(tt, "value", xRange[0].FieldValues["test"])
	})
}
