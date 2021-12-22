package tntengine

import (
	"errors"
	"fmt"
	"time"

	"github.com/FZambia/tarantool"
	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/vmihailenco/msgpack/v5"
)

// DefaultPresenceTTL is a default value for presence TTL in Tarantool.
const DefaultPresenceTTL = 60 * time.Second

// PresenceManagerConfig is a config for Tarantool-based PresenceManager.
type PresenceManagerConfig struct {
	// PresenceTTL is an interval how long to consider presence info
	// valid after receiving presence update. This allows to automatically
	// clean up unnecessary presence entries after TTL passed.
	PresenceTTL time.Duration

	// Shards is a list of Tarantool instances to shard data by channel.
	Shards []*Shard
}

// NewPresenceManager initializes Tarantool-based centrifuge.PresenceManager.
func NewPresenceManager(n *centrifuge.Node, config PresenceManagerConfig) (*PresenceManager, error) {
	if len(config.Shards) == 0 {
		return nil, errors.New("no Tarantool shards provided in configuration")
	}
	if len(config.Shards) > 1 {
		n.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, fmt.Sprintf("Tarantool sharding enabled: %d shards", len(config.Shards))))
	}
	e := &PresenceManager{
		node:     n,
		shards:   config.Shards,
		config:   config,
		sharding: len(config.Shards) > 1,
	}
	return e, nil
}

var _ centrifuge.PresenceManager = (*PresenceManager)(nil)

// PresenceManager uses Tarantool to implement centrifuge.PresenceManager functionality.
type PresenceManager struct {
	node     *centrifuge.Node
	sharding bool
	config   PresenceManagerConfig
	shards   []*Shard
}

type presenceRequest struct {
	Channel string
}

func (m *PresenceManager) Presence(ch string) (map[string]*centrifuge.ClientInfo, error) {
	s := consistentShard(ch, m.shards)
	res, err := s.Exec(tarantool.Call("centrifuge.presence", presenceRequest{Channel: ch}))
	if err != nil {
		return nil, err
	}
	if len(res.Data) == 0 {
		return nil, errors.New("malformed presence result")
	}
	presenceInterfaceSlice, ok := res.Data[0].([]interface{})
	if !ok {
		return nil, errors.New("malformed presence format: map expected")
	}
	presence := make(map[string]*centrifuge.ClientInfo, len(presenceInterfaceSlice))
	for _, v := range presenceInterfaceSlice {
		presenceRow, ok := v.([]interface{})
		if !ok {
			return nil, errors.New("malformed presence format: tuple expected")
		}
		var ci protocol.ClientInfo
		ciData, ok := presenceRow[3].(string)
		if !ok {
			return nil, errors.New("malformed presence format: data expected")
		}
		err := ci.UnmarshalVT([]byte(ciData))
		if err != nil {
			return nil, fmt.Errorf("unmarshal ClientInfo: %w", err)
		}
		presence[ci.Client] = &centrifuge.ClientInfo{
			ClientID: ci.Client,
			UserID:   ci.User,
			ConnInfo: ci.ConnInfo,
			ChanInfo: ci.ChanInfo,
		}
	}
	return presence, nil
}

type presenceStatsRequest struct {
	Channel string
}

type presenceStatsResponse struct {
	NumClients uint32
	NumUsers   uint32
}

func (m *presenceStatsResponse) DecodeMsgpack(d *msgpack.Decoder) error {
	var err error
	var l int
	if l, err = d.DecodeArrayLen(); err != nil {
		return err
	}
	if l != 2 {
		return fmt.Errorf("array len doesn't match: %d", l)
	}
	if m.NumClients, err = d.DecodeUint32(); err != nil {
		return err
	}
	if m.NumUsers, err = d.DecodeUint32(); err != nil {
		return err
	}
	return nil
}

func (m *PresenceManager) PresenceStats(ch string) (centrifuge.PresenceStats, error) {
	s := consistentShard(ch, m.shards)
	var resp presenceStatsResponse
	err := s.ExecTyped(tarantool.Call("centrifuge.presence_stats", presenceStatsRequest{Channel: ch}), &resp)
	if err != nil {
		return centrifuge.PresenceStats{}, err
	}
	return centrifuge.PresenceStats{NumClients: int(resp.NumClients), NumUsers: int(resp.NumUsers)}, err
}

type addPresenceRequest struct {
	Channel  string
	TTL      int
	ClientID string
	UserID   string
	Data     string
}

func (m *PresenceManager) AddPresence(ch string, clientID string, info *centrifuge.ClientInfo) error {
	s := consistentShard(ch, m.shards)
	ttl := DefaultPresenceTTL
	if m.config.PresenceTTL > 0 {
		ttl = m.config.PresenceTTL
	}
	_, err := s.Exec(tarantool.Call("centrifuge.add_presence", addPresenceRequest{
		Channel:  ch,
		TTL:      int(ttl.Seconds()),
		ClientID: clientID,
		UserID:   info.UserID,
		Data:     m.clientInfoString(info),
	}))
	return err
}

func (m *PresenceManager) clientInfoString(clientInfo *centrifuge.ClientInfo) string {
	var info string
	if clientInfo != nil {
		byteMessage, err := infoToProto(clientInfo).MarshalVT()
		if err != nil {
			return info
		}
		info = string(byteMessage)
	}
	return info
}

type removePresenceRequest struct {
	Channel  string
	ClientID string
}

func (m *PresenceManager) RemovePresence(ch string, clientID string) error {
	s := consistentShard(ch, m.shards)
	_, err := s.Exec(tarantool.Call("centrifuge.remove_presence", removePresenceRequest{Channel: ch, ClientID: clientID}))
	return err
}
