package redisqueue

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/redisshard"

	"github.com/redis/rueidis"
)

// ConsumerFunc is a type alias for the functions that will be used to handle
// and process Messages.
type ConsumerFunc func(*Message) error

// ConsumerOptions provide options to configure the Consumer.
type ConsumerOptions struct {
	// Stream is stream to consume.
	Stream string
	// GroupName sets the name of the consumer group. This will be used when
	// coordinating in Redis. If empty, stream name will be used.
	GroupName string
	// Name sets the name of this consumer. This will be used when fetching from
	// Redis. If empty, the hostname will be used.
	Name string

	// ConsumerFunc will be called upon task message received from Redis.
	ConsumerFunc ConsumerFunc

	// VisibilityTimeout dictates the maximum amount of time a message should
	// stay in pending. If there is a message that has been idle for more than
	// this duration, the consumer will attempt to claim it.
	VisibilityTimeout time.Duration
	// BlockingTimeout designates how long the XREADGROUP call blocks for. If
	// this is 0, it will block indefinitely. While this is the most efficient
	// from a polling perspective, if this call never times out, there is no
	// opportunity to yield back to Go at a regular interval. This means it's
	// possible that if no messages are coming in, the consumer cannot
	// gracefully shutdown. Instead, it's recommended to set this to 1-5
	// seconds, or even longer, depending on how long your application can wait
	// for shutdown.
	BlockingTimeout time.Duration
	// ReclaimInterval is the amount of time in between calls to XPENDING to
	// attempt to reclaim jobs that have been idle for more than the visibility
	// timeout. A smaller duration will result in more frequent checks. This
	// will allow messages to be reaped faster, but it will put more load on
	// Redis.
	ReclaimInterval time.Duration
	// Concurrency dictates how many goroutines to spawn to handle the messages.
	Concurrency int
	// UseLegacyReclaim enables using xpending + xclaim instead of xautoclaim.
	UseLegacyReclaim bool
}

// Consumer adds a convenient wrapper around consuming jobs and managing concurrency.
type Consumer struct {
	// Errors is a channel that you can receive from to centrally handle any
	// errors that may occur either by your ConsumerFunc or by internal
	// processing functions. Because this is an unbuffered channel, you must
	// have a listener on it. If you don't – parts of the consumer could stop
	// functioning when errors occur due to the blocking nature of unbuffered
	// channels.
	Errors chan error

	options  ConsumerOptions
	queue    chan *Message
	cond     *sync.Cond
	inflight int64

	stopReclaim chan struct{}
	stopPoll    chan struct{}
	stopWorkers chan struct{}

	shard *redisshard.RedisShard
}

// NewConsumer creates a Consumer with custom ConsumerOptions.
func NewConsumer(shard *redisshard.RedisShard, options ConsumerOptions) (*Consumer, error) {
	if options.Stream == "" {
		return nil, errors.New("stream required")
	}
	if options.ConsumerFunc == nil {
		return nil, errors.New("ConsumerFunc required")
	}
	if options.GroupName == "" {
		return nil, errors.New("GroupName required")
	}
	if options.Name == "" {
		hostname, _ := os.Hostname()
		options.Name = hostname
	}
	if options.BlockingTimeout == 0 {
		options.BlockingTimeout = 15 * time.Second
	}
	if options.ReclaimInterval == 0 {
		options.ReclaimInterval = 5 * time.Second
	}

	c := &Consumer{
		Errors: make(chan error),

		options: options,
		queue:   make(chan *Message, options.Concurrency),
		cond:    sync.NewCond(&sync.Mutex{}),

		stopReclaim: make(chan struct{}, 1),
		stopPoll:    make(chan struct{}, 1),
		stopWorkers: make(chan struct{}),

		shard: shard,
	}
	err := CreateConsumerGroup(c.shard, c.options.Stream, c.options.GroupName, "$")
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return nil, fmt.Errorf("error creating consumer group: %w", err)
	}
	return c, nil
}

func (c *Consumer) Options() ConsumerOptions {
	return c.options
}

// Run starts all the worker goroutines and starts processing from the
// streams that have been registered with Register. All errors will be sent to
// the Errors channel. If Register was never called, an error will be sent and
// Run will terminate early. The same will happen if an error occurs when
// creating the consumer group in Redis. Run will block until Shutdown is called
// and all the in-flight messages have been processed.
func (c *Consumer) Run() {
	if c.options.UseLegacyReclaim {
		go c.reclaimLegacy(c.options.Stream)
	} else {
		go c.reclaim(c.options.Stream)
	}

	var pollWg sync.WaitGroup
	pollWg.Add(1)

	go func() {
		defer pollWg.Done()
		c.poll()
	}()

	stop := newSignalHandler()
	go func() {
		<-stop
		c.Shutdown()
	}()

	var workerWg sync.WaitGroup
	workerWg.Add(c.options.Concurrency)

	for i := 0; i < c.options.Concurrency; i++ {
		go func() {
			defer workerWg.Done()
			c.work()
		}()
	}

	pollWg.Wait()
	close(c.stopWorkers)
	workerWg.Wait()
}

func (c *Consumer) InflightJobs() int64 {
	return atomic.LoadInt64(&c.inflight)
}

// Shutdown stops new messages from being processed and tells the workers to
// wait until all in-flight messages have been processed, and then they exit.
// The order that things stop is 1) the reclaim process (if it's running), 2)
// the polling process, and 3) the worker processes.
func (c *Consumer) Shutdown() {
	c.stopReclaim <- struct{}{}
	if c.options.VisibilityTimeout == 0 {
		c.stopPoll <- struct{}{}
	}
	c.cond.L.Lock()
	c.cond.Broadcast()
	c.cond.L.Unlock()
}

// reclaim runs in a separate goroutine and checks the list of pending messages
// in every stream. For every message, if it's been idle for longer than the
// VisibilityTimeout, it will attempt to claim that message for this consumer.
// If VisibilityTimeout is 0, this function returns early and no messages are
// reclaimed. It checks the list of pending messages according to
// ReclaimInterval.
func (c *Consumer) reclaim(stream string) {
	if c.options.VisibilityTimeout == 0 {
		return
	}

	ticker := time.NewTicker(c.options.ReclaimInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopReclaim:
			// once the reclaim process has stopped, stop the polling process
			c.stopPoll <- struct{}{}
			return
		case <-ticker.C:
			start := "-"
			for {
				c.cond.L.Lock()
				inflight := atomic.LoadInt64(&c.inflight)
				if inflight >= int64(c.options.Concurrency) {
					c.cond.Wait()
					c.cond.L.Unlock()
					continue
				}
				c.cond.L.Unlock()
				count := c.options.Concurrency - len(c.queue)
				if count <= 0 {
					count = 1
				}
				res := c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
					cmd := client.B().Xautoclaim().
						Key(stream).Group(c.options.GroupName).Consumer(c.options.Name).
						MinIdleTime(strconv.FormatInt(c.options.VisibilityTimeout.Milliseconds(), 10)).
						Start(start).Count(int64(count)).Build()
					return client.Do(context.Background(), cmd)
				})
				entries, newStart, err := NewXAutoClaimCmd(res).Result()
				if err != nil && !rueidis.IsRedisNil(err) {
					c.logError(fmt.Errorf("error listing pending messages: %w", err))
					break
				}

				if rueidis.IsRedisNil(err) {
					break
				}

				start = newStart

				if len(entries) > 0 {
					c.enqueue(entries)
				} else {
					break
				}
			}
		}
	}
}

// reclaimLegacy runs in a separate goroutine and checks the list of pending messages
// in every stream. For every message, if it's been idle for longer than the
// VisibilityTimeout, it will attempt to claim that message for this consumer.
// If VisibilityTimeout is 0, this function returns early and no messages are
// reclaimed. It checks the list of pending messages according to
// ReclaimInterval.
func (c *Consumer) reclaimLegacy(stream string) {
	if c.options.VisibilityTimeout == 0 {
		return
	}

	ticker := time.NewTicker(c.options.ReclaimInterval)

	for {
		select {
		case <-c.stopReclaim:
			// once the reclaim process has stopped, stop the polling process
			c.stopPoll <- struct{}{}
			return
		case <-ticker.C:
			start := "-"
			end := "+"

			for {
				res := xPendingExt(c.shard, stream, c.options.GroupName, start, end, int64(c.options.Concurrency-len(c.queue)))
				if res.Error() != nil {
					c.logError(fmt.Errorf("error getting pending messages: %w", res.Error()))
					break
				}
				xPendingRes := NewXPendingExtCmd(res)
				if xPendingRes.Err != nil {
					c.logError(fmt.Errorf("error parsing pending messages: %w", xPendingRes.Err))
					break
				}
				if len(xPendingRes.Val) == 0 {
					break
				}
				for _, r := range xPendingRes.Val {
					if r.Idle >= c.options.VisibilityTimeout {
						res = c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
							cmd := client.B().Xclaim().
								Key(stream).Group(c.options.GroupName).Consumer(c.options.Name).
								MinIdleTime(strconv.FormatInt(c.options.VisibilityTimeout.Milliseconds(), 10)).
								Id(r.ID).Build()
							return client.Do(context.Background(), cmd)
						})
						if res.Error() != nil {
							c.logError(fmt.Errorf("error claiming pending message: %w", res.Error()))
							break
						}

						claimedEntries, err := res.AsXRange()
						if err != nil {
							c.logError(fmt.Errorf("error parsing pending messages: %w", err))
							break
						}

						// If the no message returned, it means that
						// the message no longer exists in the stream.
						// However, it is still in a pending state. This
						// could happen if a message was claimed by a
						// consumer, that consumer died, and the message
						// gets deleted (either through a XDEL call or
						// through MAXLEN). Since the message no longer
						// exists, the only way we can get it out of the
						// pending state is to acknowledge it.
						if len(claimedEntries) == 0 {
							res := c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
								cmd := client.B().Xack().Key(stream).Group(c.options.GroupName).Id(r.ID).Build()
								return client.Do(context.Background(), cmd)
							})
							if res.Error() != nil {
								c.logError(fmt.Errorf("error acknowledging after failed claim for %q stream and %q message: %w", stream, r.ID, res.Error()))
								continue
							}
						}
						c.enqueue(claimedEntries)
					}
				}

				newID, err := incrementMessageID(xPendingRes.Val[len(xPendingRes.Val)-1].ID)
				if err != nil {
					c.logError(fmt.Errorf("error incrementing message ID: %w", err))
					break
				}

				start = newID
			}
		}
	}
}

func xPendingExt(shard *redisshard.RedisShard, stream, groupName, start, end string, count int64) rueidis.RedisResult {
	return shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
		cmd := client.B().Xpending().Key(stream).Group(groupName).Start(start).End(end).Count(count).Build()
		return client.Do(context.Background(), cmd)
	})
}

// incrementMessageID takes in a message ID (e.g. 1564886140363-0) and
// increments the index section (e.g. 1564886140363-1). This is the next valid
// ID value, and it can be used for paging through messages.
func incrementMessageID(id string) (string, error) {
	parts := strings.Split(id, "-")
	index := parts[1]
	parsed, err := strconv.ParseInt(index, 10, 64)
	if err != nil {
		return "", fmt.Errorf("error parsing message ID %q: %w", id, err)
	}
	return fmt.Sprintf("%s-%d", parts[0], parsed+1), nil
}

func (c *Consumer) xReadGroup(stream, groupName, name string, count int64, blockingMilliseconds int64) rueidis.RedisResult {
	return c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
		cmd := client.B().Xreadgroup().Group(groupName, name).Count(count).Block(blockingMilliseconds).Streams().Key(stream).Id(">").Build()
		return client.Do(context.Background(), cmd)
	})
}

// poll constantly checks the streams using XREADGROUP to see if there are any
// messages for this consumer to process. It blocks for up to 5 seconds instead
// of blocking indefinitely so that it can periodically check to see if Shutdown
// was called.
func (c *Consumer) poll() {
	for {
		select {
		case <-c.stopPoll:
			return
		default:
			c.cond.L.Lock()
			inflight := atomic.LoadInt64(&c.inflight)
			if inflight >= int64(c.options.Concurrency) {
				c.cond.Wait()
				c.cond.L.Unlock()
				continue
			}
			c.cond.L.Unlock()
			count := c.options.Concurrency - len(c.queue)

			res := c.xReadGroup(c.options.Stream, c.options.GroupName, c.options.Name, int64(count), c.options.BlockingTimeout.Milliseconds())
			err := res.Error()
			if err != nil {
				if err, ok := err.(net.Error); ok && err.Timeout() {
					continue
				}
				if rueidis.IsRedisNil(err) {
					continue
				}

				if strings.Contains(err.Error(), "NOGROUP") {
					err := CreateConsumerGroup(c.shard, c.options.Stream, c.options.GroupName, "$")
					if err != nil {
						c.logError(fmt.Errorf("error creating consumer group: %w", err))
					}
					continue
				}

				c.logError(fmt.Errorf("error reading redis stream %s: %w", c.options.Stream, err))
				select {
				case <-c.stopPoll:
					return
				case <-time.After(200 * time.Millisecond):
				}
				continue
			}

			xRead, err := res.AsXRead()
			if err != nil {
				c.logError(fmt.Errorf("error parsing redis stream %s: %w", c.options.Stream, err))
				continue
			}

			for _, messages := range xRead {
				c.enqueue(messages)
			}
		}
	}
}

func (c *Consumer) logError(err error) {
	select {
	case c.Errors <- err:
	default:
	}
}

// enqueue takes a slice of XMessages, creates corresponding Messages, and sends
// them on the centralized channel for worker goroutines to process.
func (c *Consumer) enqueue(messages []rueidis.XRangeEntry) {
	for _, m := range messages {
		msg := &Message{
			ID:     m.ID,
			Values: m.FieldValues,
		}
		select {
		case c.queue <- msg:
			atomic.AddInt64(&c.inflight, 1)
		case <-c.stopWorkers:
			return
		}
	}
}

// work is called in a separate goroutine. The number of work goroutines is
// determined by Concurrency. Once it gets a message from the centralized
// channel, it calls the corresponding ConsumerFunc depending on the stream it
// came from. If no error is returned from the ConsumerFunc, the message is
// acknowledged in Redis.
func (c *Consumer) work() {
	for {
		select {
		case msg := <-c.queue:
			err := c.process(msg)
			if err != nil {
				c.logError(fmt.Errorf("error calling processing func for %q stream and %q message: %w", c.options.Stream, msg.ID, err))
				continue
			}
			err = c.shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
				cmd := client.B().Xack().Key(c.options.Stream).Group(c.options.GroupName).Id(msg.ID).Build()
				return client.Do(context.Background(), cmd)
			}).Error()
			if err != nil {
				c.logError(fmt.Errorf("error acknowledging message for %q stream and %q message: %w", c.options.Stream, msg.ID, err))
				continue
			}
		case <-c.stopWorkers:
			return
		}
	}
}

func (c *Consumer) process(msg *Message) (err error) {
	defer func() {
		c.cond.L.Lock()
		atomic.AddInt64(&c.inflight, -1)
		c.cond.Broadcast()
		c.cond.L.Unlock()
	}()
	err = c.options.ConsumerFunc(msg)
	return
}
