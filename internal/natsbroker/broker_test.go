package natsbroker

import (
	"strconv"
	"testing"

	"github.com/centrifugal/centrifuge"
)

func newTestNatsBroker() *NatsBroker {
	return NewTestNatsBrokerWithPrefix("centrifuge-test")
}

func NewTestNatsBrokerWithPrefix(prefix string) *NatsBroker {
	n, _ := centrifuge.New(centrifuge.Config{})
	b, _ := New(n, Config{Prefix: prefix})
	n.SetBroker(b)
	err := n.Run()
	if err != nil {
		panic(err)
	}
	return b
}

func BenchmarkNatsEnginePublish(b *testing.B) {
	broker := newTestNatsBroker()
	rawData := centrifuge.Raw([]byte(`{"bench": true}`))
	pub := &centrifuge.Publication{UID: "test UID", Data: rawData}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := broker.Publish("channel", pub, &centrifuge.ChannelOptions{HistorySize: 0, HistoryLifetime: 0})
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkNatsEnginePublishParallel(b *testing.B) {
	broker := newTestNatsBroker()
	rawData := centrifuge.Raw([]byte(`{"bench": true}`))
	pub := &centrifuge.Publication{UID: "test UID", Data: rawData}
	b.SetParallelism(128)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := broker.Publish("channel", pub, &centrifuge.ChannelOptions{HistorySize: 0, HistoryLifetime: 0})
			if err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkNatsEngineSubscribe(b *testing.B) {
	broker := newTestNatsBroker()
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

func BenchmarkNatsEngineSubscribeParallel(b *testing.B) {
	broker := newTestNatsBroker()
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
