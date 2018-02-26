package proto

import (
	"sync"
)

// PreparedReply is structure for encoding reply only once.
type PreparedReply struct {
	Enc   Encoding
	Reply *Reply
	data  []byte
	once  sync.Once
}

// NewPreparedReply initializes PreparedReply.
func NewPreparedReply(reply *Reply, enc Encoding) *PreparedReply {
	return &PreparedReply{
		Reply: reply,
		Enc:   enc,
	}
}

// Data returns data associated with reply which is only calculated once.
func (r *PreparedReply) Data() []byte {
	r.once.Do(func() {
		encoder := GetReplyEncoder(r.Enc)
		encoder.Encode(r.Reply)
		data := encoder.Finish()
		PutReplyEncoder(r.Enc, encoder)
		r.data = data
	})
	return r.data
}
