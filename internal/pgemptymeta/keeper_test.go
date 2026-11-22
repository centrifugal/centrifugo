package pgemptymeta

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTTL(t *testing.T) {
	const check = 40 * time.Second
	for _, tt := range []struct {
		name                 string
		metaTTL, unpublished time.Duration
		want                 time.Duration
	}{
		{"option off", time.Hour, 0, time.Hour},
		{"option shorter", time.Hour, 10 * time.Minute, 10 * time.Minute},
		{"option longer", time.Hour, 2 * time.Hour, time.Hour},
		{"permanent", 0, 0, 0},
		{"permanent with option", 0, 10 * time.Minute, 10 * time.Minute},
		{"floor", time.Second, 0, 2 * time.Minute},
	} {
		require.Equal(t, tt.want, TTL(tt.metaTTL, tt.unpublished, check), tt.name)
	}
}

func TestKeeperSchedule(t *testing.T) {
	now := time.Now()
	k := New(nil, "meta", func(ch string) bool { return ch != "unwatched" })
	k.Watch("unwatched", 0, now)
	k.Watch("soon", -5, now)
	k.Watch("later", 60, now)
	k.Watch("capped", (720 * time.Hour).Seconds(), now)

	require.ElementsMatch(t, []string{"soon"}, k.DueChannels(now))
	require.ElementsMatch(t, []string{"soon", "later"}, k.DueChannels(now.Add(time.Minute)))
	require.ElementsMatch(t, []string{"soon", "later", "capped"}, k.DueChannels(now.Add(maxWait)))
	k.Forget("soon", "later")
	require.ElementsMatch(t, []string{"capped"}, k.DueChannels(now.Add(maxWait)))
}
