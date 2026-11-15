package usage

import (
	"strings"
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

func Test_getHistogramMetric(t *testing.T) {
	type args struct {
		val          int
		bounds       []int
		metricPrefix string
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			"1",
			args{0, []int{0, 10, 100, 1000, 10000, 100000}, "users."},
			"users.le_0",
		},
		{
			"2",
			args{10, []int{0, 10, 100, 1000, 10000, 100000}, "users."},
			"users.le_10",
		},
		{
			"3",
			args{30000, []int{0, 10, 100, 1000, 10000, 100000}, "users."},
			"users.le_100k",
		},
		{
			"4",
			args{3000000, []int{0, 10, 100, 1000, 10000, 100000, 10000000}, "users."},
			"users.le_10m",
		},
		{
			"5",
			args{300000, []int{0, 10, 100, 1000, 10000, 100000}, "users."},
			"users.le_inf",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getHistogramMetric(tt.args.val, tt.args.bounds, tt.args.metricPrefix); got != tt.want {
				t.Errorf("getHistogramMetric() = %v, want %v", got, tt.want)
			}
		})
	}
}

func nodeWithMemoryEngine(t *testing.T) *centrifuge.Node {
	n, err := centrifuge.New(centrifuge.Config{})
	if err != nil {
		t.Fatal(err)
	}
	err = n.Run()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPrepareMetrics(t *testing.T) {
	node := nodeWithMemoryEngine(t)
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	sender := NewSender(node, cfgContainer, Features{})
	err = sender.updateMaxValues()
	require.NoError(t, err)
	metrics, err := sender.prepareMetrics()
	require.NoError(t, err)
	require.True(t, len(metrics) > 0)
}

func TestPrepareMetricsSubscriptionTypes(t *testing.T) {
	node := nodeWithMemoryEngine(t)
	cfg := config.DefaultConfig()
	cfg.SharedPoll.HMACSecretKey = "secret"
	cfg.Channel.Proxy.SharedPollRefresh.Endpoint = "http://localhost:3000/refresh"
	cfg.Channel.WithoutNamespace.SubscriptionType = "map"
	cfg.Channel.WithoutNamespace.Map.Mode = "persistent"
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		{Name: "ns_a", ChannelOptions: configtypes.ChannelOptions{SubscriptionType: "shared_poll"}},
		{Name: "ns_b", ChannelOptions: configtypes.ChannelOptions{SubscriptionType: "map", Map: configtypes.MapConfig{Mode: "persistent"}}},
		{Name: "ns_c", ChannelOptions: configtypes.ChannelOptions{SubscriptionType: "stream"}},
		{Name: "ns_d"},
	}
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	sender := NewSender(node, cfgContainer, Features{})
	err = sender.updateMaxValues()
	require.NoError(t, err)
	metrics, err := sender.prepareMetrics()
	require.NoError(t, err)

	var subscriptionTypeMetrics []string
	for _, m := range metrics {
		if strings.HasPrefix(m.Name, metricsPrefix+"stats.subscription_type.") {
			subscriptionTypeMetrics = append(subscriptionTypeMetrics, m.Name)
		}
	}
	require.Equal(t, []string{
		metricsPrefix + "stats.subscription_type.map",
		metricsPrefix + "stats.subscription_type.shared_poll",
	}, subscriptionTypeMetrics)
}
