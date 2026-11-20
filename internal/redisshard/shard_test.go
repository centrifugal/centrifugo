package redisshard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/redis/rueidis"
	"github.com/stretchr/testify/require"
)

// Sentinel connections are dialled with the data connections' connect timeout
// and keep-alive, whether the shard is configured with SentinelAddresses or a
// redis+sentinel:// address (centrifugo#1325).
func TestRedisClientOptions_SentinelDialer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		conf RedisShardConfig
	}{
		{
			name: "sentinel addresses",
			conf: RedisShardConfig{
				SentinelAddresses:  []string{"127.0.0.1:26379"},
				SentinelMasterName: "mymaster",
			},
		},
		{
			name: "sentinel URL",
			conf: RedisShardConfig{
				Address: "redis+sentinel://127.0.0.1:26379?sentinel_master_name=mymaster&connect_timeout=3s",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			conf := tc.conf
			conf.ConnectTimeout = defaultRedisConnectTimeout
			conf.IOTimeout = defaultRedisIOTimeout
			options, _, isSentinel, _, err := redisClientOptions(conf)
			require.NoError(t, err)
			require.True(t, isSentinel)
			require.NotZero(t, options.Dialer.Timeout)
			require.NotZero(t, options.Dialer.KeepAlive)
			require.Equal(t, options.Dialer.Timeout, options.Sentinel.Dialer.Timeout)
			require.Equal(t, options.Dialer.KeepAlive, options.Sentinel.Dialer.KeepAlive)
		})
	}
	// The address's connect_timeout reaches the Sentinel connections too.
	options, _, _, _, err := redisClientOptions(RedisShardConfig{
		Address:        "redis+sentinel://127.0.0.1:26379?sentinel_master_name=mymaster&connect_timeout=3s",
		ConnectTimeout: defaultRedisConnectTimeout,
		IOTimeout:      defaultRedisIOTimeout,
	})
	require.NoError(t, err)
	require.Equal(t, 3*time.Second, options.Sentinel.Dialer.Timeout)
}

// A connect which fails with a network error is repeated until it succeeds.
func TestWaitForRedisStartupRetriesUntilConnected(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var attempts int
		connect := func() (rueidis.Client, error) {
			attempts++
			if attempts < 4 {
				return nil, errors.New("dial tcp: connection refused")
			}
			return nil, nil
		}
		_, err := waitForRedisStartup(time.Now().Add(time.Minute), connect, func(error, time.Duration) {})
		require.NoError(t, err)
		require.Equal(t, 4, attempts)
	})
}

// Redis which never comes up costs the timeout and no more, and the caller
// gets the error of the last attempt.
func TestWaitForRedisStartupReturnsLastErrorAfterTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var attempts int
		connect := func() (rueidis.Client, error) {
			attempts++
			return nil, fmt.Errorf("attempt %d failed", attempts)
		}
		started := time.Now()
		_, err := waitForRedisStartup(started.Add(30*time.Second), connect, func(error, time.Duration) {})
		require.EqualError(t, err, fmt.Sprintf("attempt %d failed", attempts))
		require.Equal(t, 30*time.Second, time.Since(started))
	})
}

// Two waits with one deadline, as for the primary and the replica client,
// take the timeout in total.
func TestWaitForRedisStartupSharesDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		deadline := started.Add(30 * time.Second)
		var attempts int
		connectLate := func() (rueidis.Client, error) {
			attempts++
			if attempts < 6 {
				return nil, errors.New("dial tcp: connection refused")
			}
			return nil, nil
		}
		_, err := waitForRedisStartup(deadline, connectLate, func(error, time.Duration) {})
		require.NoError(t, err)
		require.Positive(t, time.Since(started))

		connectNever := func() (rueidis.Client, error) {
			return nil, errors.New("dial tcp: connection refused")
		}
		_, err = waitForRedisStartup(deadline, connectNever, func(error, time.Duration) {})
		require.Error(t, err)
		require.Equal(t, 30*time.Second, time.Since(started))
	})
}

// Delays grow from the initial one to the max one, each with a jitter of up
// to a half of it.
func TestWaitForRedisStartupBacksOffWithJitter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		connect := func() (rueidis.Client, error) {
			return nil, errors.New("dial tcp: connection refused")
		}
		var delays []time.Duration
		_, err := waitForRedisStartup(time.Now().Add(time.Minute), connect, func(_ error, delay time.Duration) {
			delays = append(delays, delay)
		})
		require.Error(t, err)

		// The last delay is cut to what is left of the timeout.
		delays = delays[:len(delays)-1]
		require.Greater(t, len(delays), 8)
		maxDelay := redisStartupWaitInitialDelay
		for i, delay := range delays {
			require.GreaterOrEqual(t, delay, maxDelay/2, "delay %d", i)
			require.LessOrEqual(t, delay, maxDelay, "delay %d", i)
			maxDelay = min(2*maxDelay, redisStartupWaitMaxDelay)
		}
		require.Equal(t, redisStartupWaitMaxDelay, maxDelay)
	})
}

// An error Redis replies with, other than LOADING, comes from the
// configuration: waiting does not fix it.
func TestWaitForRedisStartupDoesNotRetryRedisError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		redisErr := (*rueidis.RedisError)(&rueidis.RedisMessage{})
		var attempts int
		connect := func() (rueidis.Client, error) {
			attempts++
			return nil, redisErr
		}
		started := time.Now()
		_, err := waitForRedisStartup(started.Add(time.Minute), connect, func(error, time.Duration) {})
		require.Equal(t, redisErr, err)
		require.Equal(t, 1, attempts)
		require.Zero(t, time.Since(started))
	})
}

// unusedTCPAddr returns an address nothing listens on.
func unusedTCPAddr(tb testing.TB) string {
	tb.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(tb, err)
	addr := ln.Addr().String()
	require.NoError(tb, ln.Close())
	return addr
}

func TestNewRedisShardFailsRightAwayWithoutStartupWaitTimeout(t *testing.T) {
	started := time.Now()
	_, err := NewRedisShard(RedisShardConfig{Address: unusedTCPAddr(t)})
	require.Error(t, err)
	require.Less(t, time.Since(started), time.Second)
}

func TestNewRedisShardFailsAfterStartupWaitTimeout(t *testing.T) {
	timeout := 500 * time.Millisecond
	started := time.Now()
	_, err := NewRedisShard(RedisShardConfig{Address: unusedTCPAddr(t), StartupWaitTimeout: timeout})
	require.Error(t, err)
	require.GreaterOrEqual(t, time.Since(started), timeout)
}

// fakeRedis speaks enough of the Redis protocol for a client to connect to it
// as to a standalone Redis, and replies to PING with a LOADING error until
// loaded is closed.
type fakeRedis struct {
	addr      string
	loaded    chan struct{}
	openConns atomic.Int64
}

func startFakeRedis(tb testing.TB) *fakeRedis {
	tb.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = ln.Close() })
	f := &fakeRedis{addr: ln.Addr().String(), loaded: make(chan struct{})}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			f.openConns.Add(1)
			go func() {
				defer f.openConns.Add(-1)
				defer func() { _ = conn.Close() }()
				r := bufio.NewReader(conn)
				for {
					command, err := readRedisCommand(r)
					if err != nil {
						return
					}
					reply := "+OK\r\n"
					if strings.EqualFold(command[0], "CLUSTER") {
						reply = "-ERR This instance has cluster support disabled\r\n"
					}
					if strings.EqualFold(command[0], "PING") {
						select {
						case <-f.loaded:
							reply = "+PONG\r\n"
						default:
							reply = "-LOADING Redis is loading the dataset in memory\r\n"
						}
					}
					if _, err := conn.Write([]byte(reply)); err != nil {
						return
					}
				}
			}()
		}
	}()
	return f
}

// readRedisCommand reads a command a client sent: an array of bulk strings.
func readRedisCommand(r *bufio.Reader) ([]string, error) {
	readLen := func(prefix byte) (int, error) {
		line, err := r.ReadString('\n')
		if err != nil {
			return 0, err
		}
		if line[0] != prefix {
			return 0, fmt.Errorf("unexpected line %q", line)
		}
		return strconv.Atoi(strings.TrimSpace(line[1:]))
	}
	n, err := readLen('*')
	if err != nil {
		return nil, err
	}
	command := make([]string, n)
	for i := range command {
		size, err := readLen('$')
		if err != nil {
			return nil, err
		}
		arg := make([]byte, size+2)
		if _, err := io.ReadFull(r, arg); err != nil {
			return nil, err
		}
		command[i] = string(arg[:size])
	}
	return command, nil
}

// Redis accepts connections while it loads a dataset but is of no use yet:
// StartupWaitTimeout waits for the load to finish too.
func TestNewRedisShardStartupWaitTimeoutWaitsForLoadingRedis(t *testing.T) {
	redis := startFakeRedis(t)
	loading := 300 * time.Millisecond
	time.AfterFunc(loading, func() { close(redis.loaded) })

	started := time.Now()
	shard, err := NewRedisShard(RedisShardConfig{
		Address:            redis.addr,
		ForceRESP2:         true,
		StartupWaitTimeout: 30 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(shard.Close)
	require.GreaterOrEqual(t, time.Since(started), loading)
	require.NoError(t, shard.client.Do(context.Background(), shard.client.B().Ping().Build()).Error())
}

// NewRedisShard which fails after it connected leaves no connection behind.
func TestNewRedisShardClosesClientOnError(t *testing.T) {
	redis := startFakeRedis(t)
	close(redis.loaded)

	_, err := NewRedisShard(RedisShardConfig{
		Address:    "redis://" + redis.addr + "?replica_client_enabled=true&force_resp2=true",
		ForceRESP2: true,
	})
	require.ErrorContains(t, err, "replica client may be initialized only in cluster and sentinel mode")
	require.Eventually(t, func() bool {
		return redis.openConns.Load() == 0
	}, 5*time.Second, 10*time.Millisecond)
}

// An attempt which is still in flight when the deadline passes is not waited
// for.
func TestWaitForRedisStartupAbandonsSlowAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		connect := func() (rueidis.Client, error) {
			time.Sleep(10 * time.Second)
			return nil, errors.New("dial tcp: i/o timeout")
		}
		started := time.Now()
		_, err := waitForRedisStartup(started.Add(time.Second), connect, func(error, time.Duration) {})
		require.ErrorIs(t, err, errRedisStartupWaitTimeout)
		require.Equal(t, time.Second, time.Since(started))

		// Let the abandoned attempt finish before the bubble ends.
		time.Sleep(10 * time.Second)
	})
}

// A server which accepts a connection and never replies holds an attempt for
// ConnectTimeout, yet NewRedisShard returns when StartupWaitTimeout passes.
func TestNewRedisShardStartupWaitTimeoutBoundsSlowAttempt(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()

	started := time.Now()
	_, err = NewRedisShard(RedisShardConfig{
		Address:            ln.Addr().String(),
		ConnectTimeout:     2 * time.Second,
		IOTimeout:          2 * time.Second,
		StartupWaitTimeout: 100 * time.Millisecond,
	})
	require.Error(t, err)
	require.Less(t, time.Since(started), time.Second)
}

// io_timeout=0s in the address means the default IO timeout for the PING of
// the wait too.
func TestNewRedisShardStartupWaitTimeoutWithZeroIOTimeoutInAddress(t *testing.T) {
	redis := startFakeRedis(t)
	close(redis.loaded)

	shard, err := NewRedisShard(RedisShardConfig{
		Address:            "redis://" + redis.addr + "?io_timeout=0s&force_resp2=true",
		StartupWaitTimeout: 500 * time.Millisecond,
	})
	require.NoError(t, err)
	shard.Close()
}
