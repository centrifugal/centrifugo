package survey

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/centrifugal/centrifugo/v3/internal/apiproto"

	"github.com/centrifugal/centrifuge"
)

// Handler can handle survey.
type Handler func(node *centrifuge.Node, data []byte) centrifuge.SurveyReply

type Caller struct {
	node     *centrifuge.Node
	handlers map[string]Handler
}

func NewCaller(node *centrifuge.Node) *Caller {
	c := &Caller{
		node: node,
		handlers: map[string]Handler{
			"channels": respondChannelsSurvey,
		},
	}
	c.node.OnSurvey(func(event centrifuge.SurveyEvent, cb centrifuge.SurveyCallback) {
		h, ok := c.handlers[event.Op]
		if !ok {
			cb(centrifuge.SurveyReply{Code: MethodNotFound})
			return
		}
		cb(h(c.node, event.Data))
	})
	return c
}

const (
	InternalError  uint32 = 1
	InvalidRequest uint32 = 2
	MethodNotFound uint32 = 3
)

func (c *Caller) Channels(ctx context.Context, _ apiproto.Raw) (apiproto.Raw, error) {
	channels, err := surveyChannels(ctx, c.node)
	if err != nil {
		return nil, err
	}
	return json.Marshal(channels)
}

func surveyChannels(ctx context.Context, node *centrifuge.Node) (map[string]int, error) {
	results, err := node.Survey(ctx, "channels", nil)
	if err != nil {
		return nil, err
	}
	channels := map[string]int{}
	for nodeID, result := range results {
		if result.Code > 0 {
			return nil, fmt.Errorf("non-zero code from node %s: %d", nodeID, result.Code)
		}
		var nodeChannels map[string]int
		err := json.Unmarshal(result.Data, &nodeChannels)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling data from node %s: %v", nodeID, err)
		}
		for ch, numSubscribers := range nodeChannels {
			channels[ch] += numSubscribers
		}
	}
	return channels, nil
}

func respondChannelsSurvey(node *centrifuge.Node, _ []byte) centrifuge.SurveyReply {
	channels := node.Hub().Channels()
	channelsMap := make(map[string]int, len(channels))
	for _, ch := range channels {
		if numSubscribers := node.Hub().NumSubscribers(ch); numSubscribers > 0 {
			channelsMap[ch] = numSubscribers
		}
	}
	data, err := json.Marshal(channelsMap)
	if err != nil {
		return centrifuge.SurveyReply{Code: InternalError}
	}
	return centrifuge.SurveyReply{Data: data}
}
