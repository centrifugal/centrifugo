package controlproto

// Encoder ...
type Encoder interface {
	EncodeCommand(*Command) ([]byte, error)
	EncodeNode(*Node) ([]byte, error)
	EncodeUnsubscribe(*Unsubscribe) ([]byte, error)
	EncodeDisconnect(*Disconnect) ([]byte, error)
}

// ProtobufEncoder ...
type ProtobufEncoder struct {
}

// NewProtobufEncoder ...
func NewProtobufEncoder() *ProtobufEncoder {
	return &ProtobufEncoder{}
}

// EncodeCommand ...
func (e *ProtobufEncoder) EncodeCommand(cmd *Command) ([]byte, error) {
	return cmd.Marshal()
}

// EncodeNode ...
func (e *ProtobufEncoder) EncodeNode(cmd *Node) ([]byte, error) {
	return cmd.Marshal()
}

// EncodeUnsubscribe ...
func (e *ProtobufEncoder) EncodeUnsubscribe(cmd *Unsubscribe) ([]byte, error) {
	return cmd.Marshal()
}

// EncodeDisconnect ...
func (e *ProtobufEncoder) EncodeDisconnect(cmd *Disconnect) ([]byte, error) {
	return cmd.Marshal()
}
