package tntengine

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/tools"

	"github.com/FZambia/tarantool"
)

const (
	defaultConnectTimeout = time.Second
	defaultRequestTimeout = time.Second
	defaultReadTimeout    = 5 * time.Second
	defaultWriteTimeout   = time.Second
)

// Shard represents single Tarantool instance.
type Shard struct {
	config ShardConfig
	subCh  chan subRequest
	mc     *MultiConnection
}

// ShardConfig allows providing options to connect to Tarantool.
type ShardConfig struct {
	// Addresses of Tarantool instances.
	Addresses []string
	// User for auth.
	User string
	// Password for auth.
	Password string
	// ConnectionMode for shard.
	ConnectionMode ConnectionMode
}

func NewShard(c ShardConfig) (*Shard, error) {
	shard := &Shard{
		config: c,
		subCh:  make(chan subRequest),
	}

	mc, err := Connect(c.Addresses, tarantool.Opts{
		ConnectTimeout: defaultConnectTimeout,
		RequestTimeout: defaultRequestTimeout,
		ReadTimeout:    defaultReadTimeout,
		WriteTimeout:   defaultWriteTimeout,
		ReconnectDelay: 50 * time.Millisecond,
		User:           c.User,
		Password:       c.Password,
		SkipSchema:     true,
	}, MultiOpts{
		ConnectionMode: c.ConnectionMode,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating connection to %#v: %w", tools.GetLogAddresses(c.Addresses), err)
	}
	shard.mc = mc
	return shard, nil
}

func (s *Shard) Exec(request *tarantool.Request) ([]interface{}, error) {
	conn, err := s.mc.LeaderConn()
	if err != nil {
		return nil, err
	}
	return conn.Exec(request)
}

func (s *Shard) ExecTyped(request *tarantool.Request, result interface{}) error {
	conn, err := s.mc.LeaderConn()
	if err != nil {
		return err
	}
	return conn.ExecTyped(request, result)
}

func (s *Shard) pubSubConn() (*tarantool.Connection, func(), error) {
	conn, err := s.mc.NewLeaderConn(tarantool.Opts{
		ConnectTimeout: defaultConnectTimeout,
		RequestTimeout: 5 * time.Second,
		ReadTimeout:    defaultReadTimeout,
		WriteTimeout:   defaultWriteTimeout,
		ReconnectDelay: 0,
		User:           s.config.User,
		Password:       s.config.Password,
		SkipSchema:     true,
	})
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				ok, err := s.mc.IsLeader(conn)
				if err != nil || !ok {
					s.mc.LeaderChanged()
					_ = conn.Close()
				}
			}
		}
	}()
	return conn, cancel, nil
}

// subRequest is an internal request to subscribe or unsubscribe from one or more channels
type subRequest struct {
	channels  []string
	subscribe bool
	err       chan error
}

// newSubRequest creates a new request to subscribe or unsubscribe form a channel.
func newSubRequest(chIDs []string, subscribe bool) subRequest {
	return subRequest{
		channels:  chIDs,
		subscribe: subscribe,
		err:       make(chan error, 1),
	}
}

// done should only be called once for subRequest.
func (sr *subRequest) done(err error) {
	sr.err <- err
}

func (sr *subRequest) result() error {
	return <-sr.err
}
