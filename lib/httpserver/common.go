package httpserver

import (
	"fmt"
	"io"

	"github.com/centrifugal/centrifugo/lib/conns"
	"github.com/centrifugal/centrifugo/lib/logging"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"
)

// common data handling logic for Websocket and Sockjs handlers.
func handleClientData(n *node.Node, c conns.Client, data []byte, transport conns.Transport, writer *writer) bool {
	if len(data) == 0 {
		n.Logger().Log(logging.NewEntry(logging.ERROR, "empty client request received"))
		transport.Close(&proto.Disconnect{Reason: proto.ErrBadRequest.Error(), Reconnect: false})
		return false
	}

	encoder := proto.GetReplyEncoder(transport.Encoding())
	decoder := proto.GetCommandDecoder(transport.Encoding(), data)

	for {
		cmd, err := decoder.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			n.Logger().Log(logging.NewEntry(logging.INFO, "error decoding request", map[string]interface{}{"client": c.ID(), "user": c.UserID(), "error": err.Error()}))
			transport.Close(proto.DisconnectBadRequest)
			proto.PutCommandDecoder(transport.Encoding(), decoder)
			proto.PutReplyEncoder(transport.Encoding(), encoder)
			return false
		}
		rep, disconnect := c.Handle(cmd)
		if disconnect != nil {
			n.Logger().Log(logging.NewEntry(logging.INFO, "disconnect after handling command", map[string]interface{}{"command": fmt.Sprintf("%v", cmd), "client": c.ID(), "user": c.UserID(), "reason": disconnect.Reason}))
			transport.Close(disconnect)
			proto.PutCommandDecoder(transport.Encoding(), decoder)
			proto.PutReplyEncoder(transport.Encoding(), encoder)
			return false
		}

		if rep != nil {
			err = encoder.Encode(rep)
			if err != nil {
				n.Logger().Log(logging.NewEntry(logging.ERROR, "error encoding reply", map[string]interface{}{"reply": fmt.Sprintf("%v", rep), "client": c.ID(), "user": c.UserID(), "error": err.Error()}))
				transport.Close(&proto.Disconnect{Reason: "internal error", Reconnect: true})
				return false
			}
		}
	}

	disconnect := writer.write(encoder.Finish())
	if disconnect != nil {
		n.Logger().Log(logging.NewEntry(logging.INFO, "disconnect after sending data to transport", map[string]interface{}{"client": c.ID(), "user": c.UserID(), "reason": disconnect.Reason}))
		transport.Close(disconnect)
		proto.PutCommandDecoder(transport.Encoding(), decoder)
		proto.PutReplyEncoder(transport.Encoding(), encoder)
		return false
	}

	proto.PutCommandDecoder(transport.Encoding(), decoder)
	proto.PutReplyEncoder(transport.Encoding(), encoder)

	return true
}
