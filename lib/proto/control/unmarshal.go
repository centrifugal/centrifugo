package control

// CommandDecoder ...
type CommandDecoder interface {
	Decode([]byte) (*Command, error)
	DecodeNode([]byte) (*Node, error)
	DecodeUnsubscribe([]byte) (*Unsubscribe, error)
	DecodeDisconnect([]byte) (*Disconnect, error)
}

// ProtobufCommandDecoder ...
type ProtobufCommandDecoder struct {
}

// NewProtobufCommandDecoder ...
func NewProtobufCommandDecoder() *ProtobufCommandDecoder {
	return &ProtobufCommandDecoder{}
}

// Decode ...
func (e *ProtobufCommandDecoder) Decode(data []byte) (*Command, error) {
	var cmd Command
	err := cmd.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

// DecodeNode ...
func (e *ProtobufCommandDecoder) DecodeNode(data []byte) (*Node, error) {
	var cmd Node
	err := cmd.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

// DecodeUnsubscribe ...
func (e *ProtobufCommandDecoder) DecodeUnsubscribe(data []byte) (*Unsubscribe, error) {
	var cmd Unsubscribe
	err := cmd.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

// DecodeDisconnect ...
func (e *ProtobufCommandDecoder) DecodeDisconnect(data []byte) (*Disconnect, error) {
	var cmd Disconnect
	err := cmd.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}
