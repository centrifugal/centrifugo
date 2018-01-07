package control

// CommandEncoder ...
type CommandEncoder interface {
	Encode(*Command) ([]byte, error)
	EncodeNode(*Node) ([]byte, error)
	EncodeUnsubscribe(*Unsubscribe) ([]byte, error)
	EncodeDisconnect(*Disconnect) ([]byte, error)
}

// ProtobufCommandEncoder ...
type ProtobufCommandEncoder struct {
}

// NewProtobufCommandEncoder ...
func NewProtobufCommandEncoder() *ProtobufCommandEncoder {
	return &ProtobufCommandEncoder{}
}

// Encode ...
func (e *ProtobufCommandEncoder) Encode(cmd *Command) ([]byte, error) {
	return cmd.Marshal()
}

// EncodeNode ...
func (e *ProtobufCommandEncoder) EncodeNode(cmd *Node) ([]byte, error) {
	return cmd.Marshal()
}

// EncodeUnsubscribe ...
func (e *ProtobufCommandEncoder) EncodeUnsubscribe(cmd *Unsubscribe) ([]byte, error) {
	return cmd.Marshal()
}

// EncodeDisconnect ...
func (e *ProtobufCommandEncoder) EncodeDisconnect(cmd *Disconnect) ([]byte, error) {
	return cmd.Marshal()
}
