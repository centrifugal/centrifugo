package api

import (
	"context"
	"strconv"
	"testing"

	. "github.com/centrifugal/centrifugo/v6/internal/apiproto"
	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/stretchr/testify/require"
)

func benchExecutor(b *testing.B) *Executor {
	b.Helper()
	node := nodeWithMemoryEngine()
	b.Cleanup(func() { _ = node.Shutdown(context.Background()) })
	cfgContainer, err := config.NewContainer(config.DefaultConfig())
	require.NoError(b, err)
	return NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test"})
}

func BenchmarkAPIPublish(b *testing.B) {
	e := benchExecutor(b)
	cmd := &PublishRequest{Channel: "test", Data: []byte(`{"k":"v"}`)}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := e.Publish(ctx, cmd)
		if resp.Error != nil {
			b.Fatal(resp.Error)
		}
	}
}

func BenchmarkAPIBroadcast(b *testing.B) {
	for _, numChannels := range []int{10, 100} {
		b.Run("channels="+strconv.Itoa(numChannels), func(b *testing.B) {
			e := benchExecutor(b)
			channels := make([]string, numChannels)
			for i := range channels {
				channels[i] = "channel" + strconv.Itoa(i)
			}
			cmd := &BroadcastRequest{Channels: channels, Data: []byte(`{"k":"v"}`)}
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				resp := e.Broadcast(ctx, cmd)
				if resp.Error != nil {
					b.Fatal(resp.Error)
				}
			}
		})
	}
}

func BenchmarkAPIBatch(b *testing.B) {
	for _, parallel := range []bool{false, true} {
		b.Run("parallel="+strconv.FormatBool(parallel), func(b *testing.B) {
			e := benchExecutor(b)
			commands := make([]*Command, 10)
			for i := range commands {
				commands[i] = &Command{Publish: &PublishRequest{Channel: "channel" + strconv.Itoa(i), Data: []byte(`{"k":"v"}`)}}
			}
			cmd := &BatchRequest{Commands: commands, Parallel: parallel}
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				resp := e.Batch(ctx, cmd)
				if resp.Replies[0].Error != nil {
					b.Fatal(resp.Replies[0].Error)
				}
			}
		})
	}
}
