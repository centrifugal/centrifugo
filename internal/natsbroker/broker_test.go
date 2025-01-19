//go:build integration

package natsbroker

import (
	"context"
	"math/rand"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

func newTestNatsBroker() *NatsBroker {
	return NewTestNatsBrokerWithPrefix("centrifugo-test")
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
	rawData := []byte(`{"bench": true}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := broker.Publish("channel", rawData, centrifuge.PublishOptions{})
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkNatsEnginePublishParallel(b *testing.B) {
	broker := newTestNatsBroker()
	rawData := []byte(`{"bench": true}`)
	b.SetParallelism(128)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, err := broker.Publish("channel", rawData, centrifuge.PublishOptions{})
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

type natsTest struct {
	Name         string
	BrokerConfig Config
	// Not all configurations support join/leave messages.
	TestJoinLeave bool
	// For WildcardChannel case we subscribe once to a wildcard channel instead of individual channels.
	WildcardChannel bool
}

var natsTests = []natsTest{
	{"default_mode", Config{}, true, false},
	{"raw_mode", Config{RawMode: configtypes.RawModeConfig{Enabled: true}}, false, false},
	{"raw_mode_wildcards", Config{AllowWildcards: true, RawMode: configtypes.RawModeConfig{Enabled: true}}, false, true},
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randString(n int) string {
	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[random.Intn(len(letterRunes))]
	}
	return string(b)
}

func getUniquePrefix() string {
	return "centrifugo-test-" + randString(3) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func stopNatsBroker(b *NatsBroker) {
	_ = b.Close(context.Background())
}

func TestNatsPubSubTwoNodes(t *testing.T) {
	for _, tt := range natsTests {
		t.Run(tt.Name, func(t *testing.T) {
			prefix := getUniquePrefix()
			tt.BrokerConfig.Prefix = prefix
			tt.BrokerConfig.RawMode.Prefix = prefix

			node1, err := centrifuge.New(centrifuge.Config{})
			require.NoError(t, err)
			b1, _ := New(node1, tt.BrokerConfig)
			node1.SetBroker(b1)
			defer func() { _ = node1.Shutdown(context.Background()) }()
			defer stopNatsBroker(b1)

			msgNum := 10
			var numPublications int64
			var numJoins int64
			var numLeaves int64
			pubCh := make(chan struct{})
			joinCh := make(chan struct{})
			leaveCh := make(chan struct{})
			brokerEventHandler := &testBrokerEventHandler{
				HandleControlFunc: func(bytes []byte) error {
					return nil
				},
				HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
					c := atomic.AddInt64(&numPublications, 1)
					if c == int64(msgNum) {
						close(pubCh)
					}
					return nil
				},
				HandleJoinFunc: func(ch string, info *centrifuge.ClientInfo) error {
					c := atomic.AddInt64(&numJoins, 1)
					if c == int64(msgNum) {
						close(joinCh)
					}
					return nil
				},
				HandleLeaveFunc: func(ch string, info *centrifuge.ClientInfo) error {
					c := atomic.AddInt64(&numLeaves, 1)
					if c == int64(msgNum) {
						close(leaveCh)
					}
					return nil
				},
			}
			_ = b1.Run(brokerEventHandler)

			if tt.WildcardChannel {
				require.NoError(t, b1.Subscribe("test.*"))
			} else {
				for i := 0; i < msgNum; i++ {
					require.NoError(t, b1.Subscribe("test."+strconv.Itoa(i)))
				}
			}

			node2, _ := centrifuge.New(centrifuge.Config{})

			b2, _ := New(node2, tt.BrokerConfig)
			node2.SetBroker(b2)
			_ = node2.Run()
			defer func() { _ = node2.Shutdown(context.Background()) }()
			defer stopNatsBroker(b2)

			for i := 0; i < msgNum; i++ {
				_, err := node2.Publish("test."+strconv.Itoa(i), []byte("123"))
				require.NoError(t, err)
				if tt.TestJoinLeave {
					err = b2.PublishJoin("test."+strconv.Itoa(i), &centrifuge.ClientInfo{})
					require.NoError(t, err)
					err = b2.PublishLeave("test."+strconv.Itoa(i), &centrifuge.ClientInfo{})
					require.NoError(t, err)
				}
			}

			select {
			case <-pubCh:
			case <-time.After(time.Second):
				require.Fail(t, "timeout waiting for PUB/SUB message")
			}
			if tt.TestJoinLeave {
				select {
				case <-joinCh:
				case <-time.After(time.Second):
					require.Fail(t, "timeout waiting for PUB/SUB join message")
				}
				select {
				case <-leaveCh:
				case <-time.After(time.Second):
					require.Fail(t, "timeout waiting for PUB/SUB leave message")
				}
			}
		})
	}
}

type testBrokerEventHandler struct {
	// Publication must register callback func to handle Publications received.
	HandlePublicationFunc func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error
	// Join must register callback func to handle Join messages received.
	HandleJoinFunc func(ch string, info *centrifuge.ClientInfo) error
	// Leave must register callback func to handle Leave messages received.
	HandleLeaveFunc func(ch string, info *centrifuge.ClientInfo) error
	// Control must register callback func to handle Control data received.
	HandleControlFunc func([]byte) error
}

func (b *testBrokerEventHandler) HandlePublication(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
	if b.HandlePublicationFunc != nil {
		return b.HandlePublicationFunc(ch, pub, sp, delta, prevPub)
	}
	return nil
}

func (b *testBrokerEventHandler) HandleJoin(ch string, info *centrifuge.ClientInfo) error {
	if b.HandleJoinFunc != nil {
		return b.HandleJoinFunc(ch, info)
	}
	return nil
}

func (b *testBrokerEventHandler) HandleLeave(ch string, info *centrifuge.ClientInfo) error {
	if b.HandleLeaveFunc != nil {
		return b.HandleLeaveFunc(ch, info)
	}
	return nil
}

func (b *testBrokerEventHandler) HandleControl(data []byte) error {
	if b.HandleControlFunc != nil {
		return b.HandleControlFunc(data)
	}
	return nil
}
