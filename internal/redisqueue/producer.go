package redisqueue

import (
	"context"
	"errors"
	"strconv"

	"github.com/centrifugal/centrifugo/v6/internal/redisshard"

	"github.com/redis/rueidis"
)

// ProducerOptions provide options to configure the Producer.
type ProducerOptions struct {
	// Stream is stream to produce into.
	Stream string
	// StreamMaxLength sets the MAXLEN option when calling XADD. This creates a
	// capped stream to prevent the stream from taking up memory indefinitely.
	// It's important to note though that this isn't the maximum number of
	// _completed_ messages, but the maximum number of _total_ messages. This
	// means that if all consumers are down, but producers are still enqueuing,
	// and the maximum is reached, unprocessed message will start to be dropped.
	// So ideally, you'll set this number to be as high as you can make it.
	// More info here: https://redis.io/commands/xadd#capped-streams.
	StreamMaxLength int64
	// ApproximateMaxLength determines whether to use the ~ with the MAXLEN
	// option. This allows the stream trimming to done in a more efficient
	// manner. More info here: https://redis.io/commands/xadd#capped-streams.
	ApproximateMaxLength bool
}

// Producer adds a convenient wrapper around enqueuing messages that will be
// processed later by a Consumer.
type Producer struct {
	shard   *redisshard.RedisShard
	options ProducerOptions
}

// NewProducer creates a Producer using custom ProducerOptions.
func NewProducer(shard *redisshard.RedisShard, options ProducerOptions) (*Producer, error) {
	if options.Stream == "" {
		return nil, errors.New("stream required")
	}
	return &Producer{
		shard:   shard,
		options: options,
	}, nil
}

// Enqueue takes in a pointer to Message and enqueues it into the stream set at
// msg.Stream. While you can set msg.ID, unless you know what you're doing, you
// should let Redis auto-generate the ID. If an ID is auto-generated, it will be
// set on msg.ID for your reference. msg.Values is also required.
func (p *Producer) Enqueue(messages ...*Message) error {
	results := p.shard.RunMulti(func(client rueidis.Client) []rueidis.RedisResult {
		commands := make(rueidis.Commands, 0, 2)
		for _, msg := range messages {
			cmd := client.B().Arbitrary("XADD").Keys(p.options.Stream)
			if p.options.StreamMaxLength > 0 {
				cmd = cmd.Args("MAXLEN", "~", strconv.FormatInt(p.options.StreamMaxLength, 10))
			}
			if msg.ID != "" {
				cmd = cmd.Args(msg.ID)
			} else {
				cmd = cmd.Args("*")
			}
			for key, value := range msg.Values {
				cmd = cmd.Args(key)
				cmd = cmd.Args(value)
			}
			commands = append(commands, cmd.Build())
		}
		return client.DoMulti(context.Background(), commands...)
	})

	for _, result := range results {
		if result.Error() != nil {
			return result.Error()
		}
	}
	for i, result := range results {
		id, err := result.ToString()
		if err != nil {
			return err
		}
		messages[i].ID = id
	}
	return nil
}
