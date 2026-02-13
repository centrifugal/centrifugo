// kafka_producer produces publish commands to a Kafka topic for Centrifugo to consume.
//
// Usage:
//
//	go run ./misc/kafka_producer/ -broker localhost:29092 -topic centrifugo -channel test -interval 1s
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

type publishRequest struct {
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data"`
}

type methodPayload struct {
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload"`
}

func main() {
	broker := flag.String("broker", "localhost:29092", "Kafka broker address")
	topic := flag.String("topic", "centrifugo", "Kafka topic to produce to")
	channel := flag.String("channel", "test", "Centrifugo channel to publish to")
	interval := flag.Duration("interval", time.Second, "Interval between messages")
	count := flag.Int("count", 0, "Number of messages to send (0 = unlimited)")
	flag.Parse()

	client, err := kgo.NewClient(kgo.SeedBrokers(*broker))
	if err != nil {
		log.Fatal("error creating Kafka client:", err)
	}
	defer client.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Ensure topic exists (use a separate client since kadm.Close closes the underlying kgo client).
	admClient, err := kgo.NewClient(kgo.SeedBrokers(*broker))
	if err != nil {
		log.Fatal("error creating admin client:", err)
	}
	adm := kadm.NewClient(admClient)
	resp, err := adm.CreateTopics(ctx, 1, 1, nil, *topic)
	if err != nil {
		log.Fatal("error creating topic:", err)
	}
	for _, t := range resp.Sorted() {
		if t.Err != nil && !strings.Contains(t.Err.Error(), "TOPIC_ALREADY_EXISTS") {
			log.Fatalf("error creating topic %s: %v", t.Topic, t.Err)
		}
		log.Printf("topic %s ready", t.Topic)
	}
	adm.Close()

	i := 0
	for {
		i++
		if *count > 0 && i > *count {
			break
		}

		data, _ := json.Marshal(map[string]interface{}{
			"message": fmt.Sprintf("hello #%d", i),
			"ts":      time.Now().UnixMilli(),
		})

		publishReq, _ := json.Marshal(publishRequest{
			Channel: *channel,
			Data:    data,
		})

		msg, _ := json.Marshal(methodPayload{
			Method:  "publish",
			Payload: publishReq,
		})

		record := &kgo.Record{
			Topic: *topic,
			Value: msg,
		}

		res := client.ProduceSync(ctx, record)
		err := res.FirstErr()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Println("error producing:", err)
		} else {
			log.Printf("produced message #%d to topic=%s channel=%s %s (offset %d)", i, *topic, *channel, string(data), res[0].Record.Offset)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(*interval):
		}
	}
}
