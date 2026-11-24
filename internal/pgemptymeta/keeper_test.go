package pgemptymeta

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTTL(t *testing.T) {
	const check = 40 * time.Second
	for _, tt := range []struct {
		name             string
		metaTTL, initial time.Duration
		want             time.Duration
	}{
		{"option off", time.Hour, 0, time.Hour},
		{"option shorter", time.Hour, 10 * time.Minute, 10 * time.Minute},
		{"option longer", time.Hour, 2 * time.Hour, time.Hour},
		{"permanent", 0, 0, 0},
		{"permanent with option", 0, 10 * time.Minute, 10 * time.Minute},
		{"option off: no floor", time.Second, 0, time.Second},
		{"floor", time.Second, time.Second, 2 * time.Minute},
	} {
		require.Equal(t, tt.want, TTL(tt.metaTTL, tt.initial, check), tt.name)
	}
}

func TestNilKeeper(t *testing.T) {
	var k *Keeper
	k.Watch("ch", 0, time.Now())
	k.Schedule("ch", 0, time.Now())
	k.Forget("ch")
}

func TestKeeperSchedule(t *testing.T) {
	now := time.Now()
	k := New(nil, "meta", func(ch string) bool { return ch != "unwatched" })
	k.Watch("unwatched", 0, now)
	k.Watch("soon", -5, now)
	k.Watch("later", 60, now)
	k.Watch("capped", (720 * time.Hour).Seconds(), now)

	require.False(t, k.IsDue("unwatched", now.Add(maxWait)))
	require.True(t, k.IsDue("soon", now))
	require.False(t, k.IsDue("later", now))
	require.True(t, k.IsDue("later", now.Add(time.Minute)))
	require.False(t, k.IsDue("capped", now.Add(time.Minute)))
	require.True(t, k.IsDue("capped", now.Add(maxWait)))
	k.Forget("soon", "later")
	require.False(t, k.IsDue("soon", now))
	require.False(t, k.IsDue("later", now.Add(time.Minute)))
}

// A subscriber came back after a run saw none, before the run dropped it.
func TestKeeperScheduleDuringSubscribersCheck(t *testing.T) {
	now := time.Now()
	var k *Keeper
	k = New(nil, "meta", func(ch string) bool {
		k.Schedule(ch, 60, now)
		return false
	})
	k.Schedule("ch", 0, now)
	require.NoError(t, k.Keep(t.Context(), now))
	require.True(t, k.IsDue("ch", now.Add(time.Minute)), "the run dropped what was scheduled during it")
}
