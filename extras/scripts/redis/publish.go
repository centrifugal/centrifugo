package main

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyburd/redigo/redis"
)

func newPool(server string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server)
			if err != nil {
				return nil, err
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
}

func main() {
	num, _ := strconv.Atoi(os.Args[1])
	batch, _ := strconv.Atoi(os.Args[2])
	fmt.Printf("%#v\n", num)

	channels := []string{}

	for i := 0; i < num; i++ {
		channels = append(channels, strconv.Itoa(i))
	}

	var message = `{"method": "publish", "params": {"channel": "test", "data": {"benchmarking": true}}}`
	messageBytes := []byte(message)

	pool := newPool(":6379")

	var counter int32

	go func() {
		prevCounter := counter
		for {
			time.Sleep(time.Second)
			current := atomic.LoadInt32(&counter)
			fmt.Printf("%d messages per seconds published\r", current-prevCounter)
			prevCounter = current
		}
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c := pool.Get()
		defer c.Close()
		j := 0
		for {
			for i := 0; i < batch; i++ {
				c.Send("PUBLISH", strconv.Itoa(j%num), messageBytes)
				j++
			}
			atomic.AddInt32(&counter, int32(batch))
			c.Flush()
			for i := 0; i < batch; i++ {
				_, err := c.Receive()
				if err != nil {
					panic(err)
				}
			}
		}
	}()
	wg.Wait()
}
