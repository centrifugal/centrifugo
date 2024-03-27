package pulsarbroker

import (
	"context"
	"strconv"
	"testing"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/centrifugal/centrifuge"
)

func newTestPulsarBroker() *PulsarBroker {
	return NewTestPulsarBrokerWithPrefix("centrifuge-test")
}

func NewTestPulsarBrokerWithPrefix(prefix string) *PulsarBroker {
	n, _ := centrifuge.New(centrifuge.Config{})
	b, _ := New(n, Config{Prefix: prefix})
	n.SetBroker(b)
	err := n.Run()
	if err != nil {
		panic(err)
	}
	return b
}

func BenchmarkPulsarEnginePublish(b *testing.B) {
	broker := newTestPulsarBroker()
	rawData := []byte(`{"bench": true}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		producer, err := broker.pc.CreateProducer(pulsar.ProducerOptions{
			Topic: "channel",
		})

		if err != nil {
			panic(err)
		}

		defer producer.Close()

		_, err = producer.Send(context.Background(), &pulsar.ProducerMessage{
			Payload: rawData,
		})

		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkPulsarEnginePublishParallel(b *testing.B) {
	broker := newTestPulsarBroker()
	rawData := []byte(`{"bench": true}`)
	b.SetParallelism(128)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			producer, err := broker.pc.CreateProducer(pulsar.ProducerOptions{
				Topic: "channel",
			})

			if err != nil {
				panic(err)
			}

			defer producer.Close()

			_, err = producer.Send(context.Background(), &pulsar.ProducerMessage{
				Payload: rawData,
			})

			if err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkPulsarEngineSubscribe(b *testing.B) {
	broker := newTestPulsarBroker()
	j := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		j++

		err := broker.Subscribe("subscribe" + strconv.Itoa(j))
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkPulsarEngineSubscribeParallel(b *testing.B) {
	broker := newTestPulsarBroker()
	i := 0
	b.SetParallelism(128)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i++

			err := broker.Subscribe("subscribe" + strconv.Itoa(i))

			if err != nil {
				panic(err)
			}
		}
	})
}
