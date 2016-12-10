package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/garyburd/redigo/redis"
)

func main() {
	n := os.Args[1]
	num, _ := strconv.Atoi(n)
	queues := os.Args[2:]
	fmt.Printf("%#v\n", num)
	fmt.Printf("%#v\n", queues)

	command := map[string]interface{}{
		"method": "publish",
		"params": map[string]interface{}{
			"channel": "test",
			"data":    map[string]bool{"benchmarking": true},
		},
	}

	bytes, _ := json.Marshal(command)
	println(string(bytes))

	var wg sync.WaitGroup
	for _, key := range queues {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			c, err := redis.Dial("tcp", ":6379")
			if err != nil {
				panic(err)
			}
			for i := 0; i < num; i++ {
				c.Send("RPUSH", key, string(bytes))
			}
			c.Flush()
		}(key)
	}
	wg.Wait()
}
