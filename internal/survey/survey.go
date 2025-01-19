package survey

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/apiproto"

	"github.com/centrifugal/centrifuge"
	"github.com/gobwas/glob"
	"google.golang.org/protobuf/proto"
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

func (c *Caller) Channels(ctx context.Context, cmd *apiproto.ChannelsRequest) (map[string]*apiproto.ChannelInfo, error) {
	return surveyChannels(ctx, c.node, cmd)
}

func surveyChannels(ctx context.Context, node *centrifuge.Node, cmd *apiproto.ChannelsRequest) (map[string]*apiproto.ChannelInfo, error) {
	req, _ := proto.Marshal(cmd)
	results, err := node.Survey(ctx, "channels", req, "")
	if err != nil {
		return nil, err
	}
	channels := map[string]*apiproto.ChannelInfo{}
	for nodeID, result := range results {
		if result.Code > 0 {
			return nil, fmt.Errorf("non-zero code from node %s: %d", nodeID, result.Code)
		}
		var nodeChannels apiproto.ChannelsResult
		err := proto.Unmarshal(result.Data, &nodeChannels)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling data from node %s: %v", nodeID, err)
		}
		for ch, chInfo := range nodeChannels.Channels {
			info, ok := channels[ch]
			if !ok {
				channels[ch] = &apiproto.ChannelInfo{NumClients: chInfo.NumClients}
			} else {
				info.NumClients += chInfo.NumClients
				channels[ch] = info
			}
		}
	}
	return channels, nil
}

func respondChannelsSurvey(node *centrifuge.Node, params []byte) centrifuge.SurveyReply {
	var req apiproto.ChannelsRequest
	err := proto.Unmarshal(params, &req)
	if err != nil {
		return centrifuge.SurveyReply{Code: InvalidRequest}
	}
	var g glob.Glob
	if req.Pattern != "" {
		var err error
		g, err = glob.Compile(req.Pattern)
		if err != nil {
			return centrifuge.SurveyReply{Code: InvalidRequest}
		}
	}
	channels := node.Hub().Channels()
	channelsMap := make(map[string]*apiproto.ChannelInfo, len(channels))
	for _, ch := range channels {
		if g != nil && !g.Match(ch) {
			continue
		}
		if numSubscribers := node.Hub().NumSubscribers(ch); numSubscribers > 0 {
			channelsMap[ch] = &apiproto.ChannelInfo{
				NumClients: uint32(numSubscribers),
			}
		}
	}
	data, err := proto.Marshal(&apiproto.ChannelsResult{Channels: channelsMap})
	if err != nil {
		return centrifuge.SurveyReply{Code: InternalError}
	}
	return centrifuge.SurveyReply{Data: data}
}
