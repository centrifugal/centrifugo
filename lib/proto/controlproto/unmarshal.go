package controlproto

// Decoder ...
type Decoder interface {
	DecodeCommand([]byte) (*Command, error)
	DecodeNode([]byte) (*Node, error)
	DecodeUnsubscribe([]byte) (*Unsubscribe, error)
	DecodeDisconnect([]byte) (*Disconnect, error)
}

// ProtobufDecoder ...
type ProtobufDecoder struct {
}

// NewProtobufDecoder ...
func NewProtobufDecoder() *ProtobufDecoder {
	return &ProtobufDecoder{}
}

// DecodeCommand ...
func (e *ProtobufDecoder) DecodeCommand(data []byte) (*Command, error) {
	var cmd Command
	err := cmd.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

// DecodeNode ...
func (e *ProtobufDecoder) DecodeNode(data []byte) (*Node, error) {
	var cmd Node
	err := cmd.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

// DecodeUnsubscribe ...
func (e *ProtobufDecoder) DecodeUnsubscribe(data []byte) (*Unsubscribe, error) {
	var cmd Unsubscribe
	err := cmd.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

// DecodeDisconnect ...
func (e *ProtobufDecoder) DecodeDisconnect(data []byte) (*Disconnect, error) {
	var cmd Disconnect
	err := cmd.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}
