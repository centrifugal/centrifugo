package tntengine

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/FZambia/tarantool"
)

type ConnectionMode int

const (
	ConnectionModeSingleInstance     ConnectionMode = 0
	ConnectionModeLeaderFollower     ConnectionMode = 1
	ConnectionModeLeaderFollowerRaft ConnectionMode = 2
)

type MultiConnection struct {
	opts       MultiOpts
	leaderMu   sync.RWMutex
	leaderAddr string
	conns      map[string]*tarantool.Connection
	closeCh    chan struct{}
	closeOnce  sync.Once
}

type MultiOpts struct {
	ConnectionMode      ConnectionMode
	LeaderCheckInterval time.Duration
}

func Connect(addrs []string, opts tarantool.Opts, multiOpts MultiOpts) (*MultiConnection, error) {
	conns, err := getConns(addrs, opts)
	if err != nil {
		return nil, err
	}
	mc := &MultiConnection{
		opts:    multiOpts,
		conns:   conns,
		closeCh: make(chan struct{}),
	}
	leaderFound := mc.checkLeaderOnce()
	if !leaderFound {
		return nil, ErrNoLeader
	}
	go mc.checkLeader()
	return mc, nil
}

var ErrNoLeader = errors.New("no leader")

func (c *MultiConnection) NewLeaderConn(opts tarantool.Opts) (*tarantool.Connection, error) {
	c.leaderMu.RLock()
	if c.leaderAddr == "" {
		c.leaderMu.RUnlock()
		return nil, ErrNoLeader
	}
	c.leaderMu.RUnlock()
	return tarantool.Connect(c.leaderAddr, opts)
}

func (c *MultiConnection) LeaderChanged() {
	if c.opts.ConnectionMode == ConnectionModeSingleInstance {
		return
	}
	c.leaderMu.Lock()
	defer c.leaderMu.Unlock()
	c.leaderAddr = ""
}

func (c *MultiConnection) LeaderConn() (*tarantool.Connection, error) {
	c.leaderMu.RLock()
	defer c.leaderMu.RUnlock()
	if c.leaderAddr != "" {
		return c.conns[c.leaderAddr], nil
	}
	return nil, ErrNoLeader
}

func getConns(addrs []string, opts tarantool.Opts) (map[string]*tarantool.Connection, error) {
	conns := map[string]*tarantool.Connection{}
	var wg sync.WaitGroup
	var connsMu sync.Mutex
	var firstErr error
	var numErrors int
	wg.Add(len(addrs))
	for _, addr := range addrs {
		go func(addr string) {
			defer wg.Done()
			conn, err := tarantool.Connect(addr, opts)
			if err != nil {
				connsMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				numErrors++
				connsMu.Unlock()
				return
			}
			connsMu.Lock()
			conns[addr] = conn
			connsMu.Unlock()
		}(addr)

	}
	wg.Wait()
	if numErrors == len(addrs) {
		return nil, firstErr
	}
	return conns, nil
}

func (c *MultiConnection) checkLeader() {
	if c.opts.ConnectionMode == ConnectionModeSingleInstance {
		return
	}
	checkInterval := c.opts.LeaderCheckInterval
	if checkInterval == 0 {
		checkInterval = time.Second
	}
	for {
		select {
		case <-c.closeCh:
			return
		case <-time.After(checkInterval):
			c.checkLeaderOnce()
		}
	}
}

func (c *MultiConnection) IsLeader(conn *tarantool.Connection) (bool, error) {
	if c.opts.ConnectionMode == ConnectionModeSingleInstance {
		return true, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	leaderCheck := "return box.info.ro == false"
	if c.opts.ConnectionMode == ConnectionModeLeaderFollowerRaft {
		leaderCheck = "return box.info.election.state == 'leader'"
	}
	resp, err := conn.ExecContext(ctx, tarantool.Eval(leaderCheck, []interface{}{}))
	if err != nil {
		return false, err
	}
	if len(resp.Data) < 1 {
		return false, errors.New("unexpected leader check result")
	}
	isLeader, ok := resp.Data[0].(bool)
	if !ok {
		return false, errors.New("malformed leader check result")
	}
	return isLeader, nil
}

func (c *MultiConnection) checkLeaderOnce() bool {
	for addr, conn := range c.conns {
		if len(c.conns) == 1 {
			c.leaderAddr = addr
			return true
		}
		isLeader, err := c.IsLeader(conn)
		if err != nil {
			continue
		}
		if isLeader {
			c.leaderMu.Lock()
			c.leaderAddr = addr
			c.leaderMu.Unlock()
			return true
		}
	}
	return false
}

func (c *MultiConnection) Close() error {
	c.closeOnce.Do(func() {
		close(c.closeCh)
		for _, conn := range c.conns {
			_ = conn.Close()
		}
	})
	return nil
}
