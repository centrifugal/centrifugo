package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/garyburd/redigo/redis"
)

func main() {
	queues := os.Args[1:]

	var wg sync.WaitGroup
	for _, key := range queues {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			c, err := redis.Dial("tcp", ":6379")
			if err != nil {
				panic(err)
			}
			var last int
			for {
				resp, err := redis.Int(c.Do("LLEN", key))
				if err != nil {
					panic(err)
				}
				if last != 0 {
					fmt.Printf("%s: %d per second\n", key, last-resp)
				}
				last = resp
				time.Sleep(time.Second)
			}
		}(key)
	}
	wg.Wait()
}
