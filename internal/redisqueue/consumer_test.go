//go:build integration

package redisqueue

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/redisshard"

	"github.com/redis/rueidis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var useLegacyReclaim = false

func testRedisShard(t *testing.T) *redisshard.RedisShard {
	t.Helper()
	shard, err := redisshard.NewRedisShard(redisshard.RedisShardConfig{
		Address: "localhost:6379",
	})
	require.NoError(t, err)
	return shard
}

func TestNewConsumer(t *testing.T) {
	t.Run("creates a new consumer", func(tt *testing.T) {
		e := testRedisShard(t)
		defer e.Close()
		c, err := NewConsumer(e, ConsumerOptions{
			Stream:    tt.Name(),
			GroupName: tt.Name(),
			ConsumerFunc: func(message *Message) error {
				return nil
			},
		})
		require.NoError(tt, err)
		assert.NotNil(tt, c)
	})
}

func TestNewConsumerWithOptions(t *testing.T) {
	t.Run("sets defaults for Name, GroupName, BlockingTimeout, and ReclaimTimeout", func(tt *testing.T) {
		e := testRedisShard(t)
		defer e.Close()
		c, err := NewConsumer(e, ConsumerOptions{
			Stream:    tt.Name(),
			GroupName: tt.Name(),
			ConsumerFunc: func(message *Message) error {
				return nil
			},
		})
		require.NoError(tt, err)

		hostname, err := os.Hostname()
		require.NoError(tt, err)

		assert.Equal(tt, hostname, c.options.Name)
		assert.Equal(tt, tt.Name(), c.options.GroupName)
		assert.Equal(tt, 15*time.Second, c.options.BlockingTimeout)
		assert.Equal(tt, 5*time.Second, c.options.ReclaimInterval)
	})

	t.Run("allows override of Name, GroupName, BlockingTimeout, ReclaimTimeout, and RedisClient", func(tt *testing.T) {
		e := testRedisShard(t)
		defer e.Close()

		c, err := NewConsumer(e, ConsumerOptions{
			Stream:    tt.Name(),
			GroupName: "test_group_name",
			Name:      "test_name",

			ConsumerFunc: func(message *Message) error {
				return nil
			},

			BlockingTimeout: 10 * time.Second,
			ReclaimInterval: 10 * time.Second,
		})
		require.NoError(tt, err)

		assert.Equal(tt, "test_name", c.options.Name)
		assert.Equal(tt, "test_group_name", c.options.GroupName)
		assert.Equal(tt, 10*time.Second, c.options.BlockingTimeout)
		assert.Equal(tt, 10*time.Second, c.options.ReclaimInterval)
	})
}

func TestRun(t *testing.T) {
	t.Run("returns an error if no ConsumerFunc are registered", func(tt *testing.T) {
		e := testRedisShard(t)
		defer e.Close()
		_, err := NewConsumer(e, ConsumerOptions{
			Stream: tt.Name(),
		})
		require.Error(tt, err)
	})

	t.Run("calls the ConsumerFunc on for a message", func(tt *testing.T) {
		e := testRedisShard(t)
		defer e.Close()

		done := make(chan struct{})

		c, err := NewConsumer(e, ConsumerOptions{
			Stream:    tt.Name(),
			GroupName: tt.Name(),
			ConsumerFunc: func(m *Message) error {
				require.Equal(tt, "value", m.Values["test"])
				close(done)
				return nil
			},

			VisibilityTimeout: 60 * time.Second,
			BlockingTimeout:   10 * time.Millisecond,
			Concurrency:       10,
		})
		require.NoError(tt, err)

		// create a producer
		p, err := NewProducer(e, ProducerOptions{
			Stream: tt.Name(),
		})
		require.NoError(tt, err)

		// create consumer group
		c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
			cmd := client.B().XgroupDestroy().Key(tt.Name()).Group(c.options.GroupName).Build()
			return client.Do(context.Background(), cmd)
		})

		res := c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
			cmd := client.B().XgroupCreate().Key(tt.Name()).Group(c.options.GroupName).Id("$").Mkstream().Build()
			return client.Do(context.Background(), cmd)
		})
		require.NoError(tt, res.Error())

		// enqueue a message
		err = p.Enqueue(&Message{
			Values: map[string]string{"test": "value"},
		})
		require.NoError(tt, err)

		// watch for consumer errors
		go func() {
			select {
			case <-done:
				return
			case err := <-c.Errors:
				require.NoError(tt, err)
			}
		}()

		// Run the consumer.
		go c.Run()

		<-done
		c.Shutdown()
	})

	t.Run("reclaims pending messages according to ReclaimInterval", func(tt *testing.T) {
		// create a consumer
		e := testRedisShard(t)
		defer e.Close()

		c, err := NewConsumer(e, ConsumerOptions{
			Stream:    tt.Name(),
			GroupName: tt.Name(),
			ConsumerFunc: func(message *Message) error {
				return nil
			},

			VisibilityTimeout: 5 * time.Millisecond,
			BlockingTimeout:   10 * time.Millisecond,
			ReclaimInterval:   1 * time.Millisecond,
			Concurrency:       10,

			UseLegacyReclaim: useLegacyReclaim,
		})
		require.NoError(tt, err)

		// create a producer
		p, err := NewProducer(e, ProducerOptions{
			Stream: tt.Name(),
		})
		require.NoError(tt, err)

		// create consumer group
		c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
			cmd := client.B().XgroupDestroy().Key(tt.Name()).Group(c.options.GroupName).Build()
			return client.Do(context.Background(), cmd)
		})

		res := c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
			cmd := client.B().XgroupCreate().Key(tt.Name()).Group(c.options.GroupName).Id("$").Mkstream().Build()
			return client.Do(context.Background(), cmd)
		})
		require.NoError(tt, res.Error())

		// enqueue a message
		msg := &Message{
			Values: map[string]string{"test": "value"},
		}
		err = p.Enqueue(msg)
		require.NoError(tt, err)

		// register a handler that will assert the message and then shut down
		// the consumer.
		c.options.ConsumerFunc = func(m *Message) error {
			assert.Equal(tt, msg.ID, m.ID)
			c.Shutdown()
			return nil
		}

		res = c.xReadGroup(tt.Name(), c.options.GroupName, "failed_consumer", 1, 1000)
		err = res.Error()
		require.NoError(tt, err)

		xRead, err := res.AsXRead()
		require.NoError(tt, err)

		require.Len(tt, xRead, 1)
		for s, messages := range xRead {
			require.Equal(tt, tt.Name(), s)
			require.Len(tt, messages, 1)
			require.Equal(tt, msg.ID, messages[0].ID)
		}

		// wait for more than VisibilityTimeout + ReclaimInterval to ensure that
		// the pending message is reclaimed
		time.Sleep(6 * time.Millisecond)

		// watch for consumer errors
		go func() {
			err := <-c.Errors
			require.NoError(tt, err)
		}()

		// run the consumer
		c.Run()
	})

	t.Run("doesn't reclaim if there is no VisibilityTimeout set", func(tt *testing.T) {
		// create a consumer
		e := testRedisShard(t)
		defer e.Close()

		c, err := NewConsumer(e, ConsumerOptions{
			Stream:    tt.Name(),
			GroupName: tt.Name(),
			ConsumerFunc: func(message *Message) error {
				return nil
			},

			BlockingTimeout: 10 * time.Millisecond,
			ReclaimInterval: 1 * time.Millisecond,
			Concurrency:     10,

			UseLegacyReclaim: useLegacyReclaim,
		})
		require.NoError(tt, err)

		// create a producer
		p, err := NewProducer(e, ProducerOptions{
			Stream:               tt.Name(),
			StreamMaxLength:      2,
			ApproximateMaxLength: false,
		})
		require.NoError(tt, err)

		// create consumer group
		c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
			cmd := client.B().XgroupDestroy().Key(tt.Name()).Group(c.options.GroupName).Build()
			return client.Do(context.Background(), cmd)
		})

		res := c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
			cmd := client.B().XgroupCreate().Key(tt.Name()).Group(c.options.GroupName).Id("$").Mkstream().Build()
			return client.Do(context.Background(), cmd)
		})
		require.NoError(tt, res.Error())

		// enqueue a message
		msg1 := &Message{
			Values: map[string]string{"test": "value"},
		}
		msg2 := &Message{
			Values: map[string]string{"test": "value2"},
		}
		err = p.Enqueue(msg1)
		require.NoError(tt, err)

		// register a handler that will assert the message and then shut down
		// the consumer
		c.options.ConsumerFunc = func(m *Message) error {
			assert.Equal(tt, msg2.ID, m.ID)
			c.Shutdown()
			return nil
		}

		res = c.xReadGroup(tt.Name(), c.options.GroupName, "failed_consumer", 1, 1000)
		err = res.Error()
		require.NoError(tt, err)

		xRead, err := res.AsXRead()
		require.NoError(tt, err)

		require.Len(tt, xRead, 1)
		for s, messages := range xRead {
			require.Equal(tt, tt.Name(), s)
			require.Len(tt, messages, 1)
			require.Equal(tt, msg1.ID, messages[0].ID)
		}

		// add another message to the stream to let the consumer consume it
		err = p.Enqueue(msg2)
		require.NoError(tt, err)

		// watch for consumer errors
		go func() {
			err := <-c.Errors
			require.NoError(tt, err)
		}()

		// run the consumer
		c.Run()

		res = xPendingExt(c.shard, tt.Name(), c.options.GroupName, "-", "+", 1)
		err = res.Error()
		require.NoError(tt, err)

		xPendingCmd := NewXPendingExtCmd(res)
		require.NoError(tt, xPendingCmd.Err)
		require.Len(tt, xPendingCmd.Val, 1)
		require.Equal(tt, msg1.ID, xPendingCmd.Val[0].ID)
	})

	t.Run("acknowledges pending messages that have already been deleted", func(tt *testing.T) {
		// create a consumer
		e := testRedisShard(t)
		defer e.Close()

		c, err := NewConsumer(e, ConsumerOptions{
			Stream:    tt.Name(),
			GroupName: tt.Name(),
			ConsumerFunc: func(message *Message) error {
				return nil
			},

			VisibilityTimeout: 5 * time.Millisecond,
			BlockingTimeout:   10 * time.Millisecond,
			ReclaimInterval:   1 * time.Millisecond,
			Concurrency:       10,

			UseLegacyReclaim: useLegacyReclaim,
		})
		require.NoError(tt, err)

		// create a producer
		p, err := NewProducer(e, ProducerOptions{
			Stream:               tt.Name(),
			StreamMaxLength:      1,
			ApproximateMaxLength: false,
		})
		require.NoError(tt, err)

		// create consumer group
		c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
			cmd := client.B().XgroupDestroy().Key(tt.Name()).Group(c.options.GroupName).Build()
			return client.Do(context.Background(), cmd)
		})

		res := c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
			cmd := client.B().XgroupCreate().Key(tt.Name()).Group(c.options.GroupName).Id("$").Mkstream().Build()
			return client.Do(context.Background(), cmd)
		})
		require.NoError(tt, res.Error())

		// enqueue a message
		msg := &Message{
			Values: map[string]string{"test": "value"},
		}
		err = p.Enqueue(msg)
		require.NoError(tt, err)

		// register a noop handler that should never be called
		c.options.ConsumerFunc = func(m *Message) error {
			t.Fail()
			return nil
		}

		// read the message but don't acknowledge it
		res = c.xReadGroup(tt.Name(), c.options.GroupName, "failed_consumer", 1, 1000)
		err = res.Error()
		require.NoError(tt, err)

		xRead, err := res.AsXRead()
		require.NoError(tt, err)

		require.Len(tt, xRead, 1)
		for s, messages := range xRead {
			require.Equal(tt, tt.Name(), s)
			require.Len(tt, messages, 1)
			require.Equal(tt, msg.ID, messages[0].ID)
		}

		// delete the message
		err = c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
			cmd := client.B().Xdel().Key(tt.Name()).Id(msg.ID).Build()
			return client.Do(context.Background(), cmd)
		}).Error()
		require.NoError(tt, err)

		// watch for consumer errors
		go func() {
			err := <-c.Errors
			require.NoError(tt, err)
		}()

		// in 10ms, shut down the consumer
		go func() {
			time.Sleep(10 * time.Millisecond)
			c.Shutdown()
		}()

		// run the consumer
		c.Run()

		res = xPendingExt(c.shard, tt.Name(), c.options.GroupName, "-", "+", 1)
		err = res.Error()
		require.NoError(tt, err)

		xPendingCmd := NewXPendingExtCmd(res)
		require.NoError(tt, xPendingCmd.Err)
		require.Len(tt, xPendingCmd.Val, 0)
	})
}
