package proto

import (
	"sync"
)

// PreparedReply ...
type PreparedReply struct {
	Enc   Encoding
	Reply *Reply
	data  []byte
	once  sync.Once
}

// NewPreparedReply ...
func NewPreparedReply(reply *Reply, enc Encoding) *PreparedReply {
	return &PreparedReply{
		Reply: reply,
		Enc:   enc,
	}
}

// Data ...
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
