package engineredis

import (
	"errors"
	"fmt"
	"hash/fnv"

	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"

	"github.com/garyburd/redigo/redis"
)

func mapStringClientInfo(result interface{}, err error) (map[string]*proto.ClientInfo, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	if len(values)%2 != 0 {
		return nil, errors.New("mapStringClientInfo expects even number of values result")
	}
	m := make(map[string]*proto.ClientInfo, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, okKey := values[i].([]byte)
		value, okValue := values[i+1].([]byte)
		if !okKey || !okValue {
			return nil, errors.New("ScanMap key not a bulk string value")
		}
		var f proto.ClientInfo
		err = f.Unmarshal(value)
		if err != nil {
			return nil, errors.New("can not unmarshal value to ClientInfo")
		}
		m[string(key)] = &f
	}
	return m, nil
}

func sliceOfMessages(n *node.Node, result interface{}, err error) ([]*proto.Publication, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	msgs := make([]*proto.Publication, len(values))
	for i := 0; i < len(values); i++ {
		value, okValue := values[i].([]byte)
		if !okValue {
			return nil, errors.New("error getting Message value")
		}

		msg, err := n.MessageDecoder().Decode(value)
		if err != nil {
			return nil, fmt.Errorf("can not unmarshal value to Message: %v", err)
		}

		if msg.Type != proto.MessageTypePublication {
			return nil, fmt.Errorf("wrong message type in history: %d", msg.Type)
		}

		publication, err := n.MessageDecoder().DecodePublication(msg.Data)
		if err != nil {
			return nil, fmt.Errorf("can not unmarshal value to Publication: %v", err)
		}
		msgs[i] = publication
	}
	return msgs, nil
}

// consistentIndex is an adapted function from https://github.com/dgryski/go-jump
// package by Damian Gryski. It consistently chooses a hash bucket number in the
// range [0, numBuckets) for the given string. numBuckets must be >= 1.
func consistentIndex(s string, numBuckets int) int {

	hash := fnv.New64a()
	hash.Write([]byte(s))
	key := hash.Sum64()

	var b int64 = -1
	var j int64

	for j < int64(numBuckets) {
		b = j
		key = key*2862933555777941757 + 1
		j = int64(float64(b+1) * (float64(int64(1)<<31) / float64((key>>33)+1)))
	}

	return int(b)
}

// index chooses bucket number in range [0, numBuckets).
func index(s string, numBuckets int) int {
	hash := fnv.New64a()
	hash.Write([]byte(s))
	return int(hash.Sum64() % uint64(numBuckets))
}
