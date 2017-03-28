package main

import (
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyburd/redigo/redis"
)

// Hash is consistent hash func using jump algorithm.
func Hash(ch string, numBuckets int) int32 {

	hash := fnv.New64a()
	hash.Write([]byte(ch))
	key := hash.Sum64()

	var b int64 = -1
	var j int64

	for j < int64(numBuckets) {
		b = j
		key = key*2862933555777941757 + 1
		j = int64(float64(b+1) * (float64(int64(1)<<31) / float64((key>>33)+1)))
	}

	return int32(b)
}

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
	numShards, _ := strconv.Atoi(os.Args[2])
	numWorkers, _ := strconv.Atoi(os.Args[3])
	fmt.Printf("%#v\n", num)

	channels := []interface{}{}

	for i := 0; i < num; i++ {
		channels = append(channels, strconv.Itoa(i))
	}

	pool := newPool(":6379")

	var counter int32

	go func() {
		prevCounter := counter
		for {
			time.Sleep(time.Second)
			current := atomic.LoadInt32(&counter)
			fmt.Printf("%d messages per seconds received\r", current-prevCounter)
			prevCounter = current
		}
	}()

	workers := map[int]chan []byte{}
	for i := 0; i < numWorkers; i++ {
		workerCh := make(chan []byte, 4096)
		workers[i] = workerCh
		go func(ch chan []byte) {
			for {
				<-ch
				//time.Sleep(20000 * time.Nanosecond)
				atomic.AddInt32(&counter, 1)
			}
		}(workerCh)
	}

	var wg sync.WaitGroup
	for i := 0; i < numShards; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			conn := redis.PubSubConn{Conn: pool.Get()}
			defer conn.Close()

			for _, ch := range channels {
				if int(Hash(ch.(string), numShards)) == i {
					fmt.Printf("channel %s goes to shard %d\n", ch.(string), i)
					err := conn.Subscribe(ch)
					if err != nil {
						panic(err)
					}
				}
			}

			for {
				switch n := conn.Receive().(type) {
				case redis.Message:
					j := int(Hash(n.Channel, numWorkers))
					workers[j] <- n.Data
					//time.Sleep(20000 * time.Nanosecond)
				case redis.Subscription:
				case error:
					panic(n)
				}
			}
		}(i)
	}
	wg.Wait()
}
