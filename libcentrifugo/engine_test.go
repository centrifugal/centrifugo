package libcentrifugo

type testEngine struct{}

func newTestEngine() *testEngine {
	return &testEngine{}
}

func (e *testEngine) name() string {
	return "test engine"
}

func (e *testEngine) run() error {
	return nil
}

func (e *testEngine) publishMessage(ch Channel, message *Message, opts *ChannelOptions) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *testEngine) publishJoin(ch Channel, message *JoinMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *testEngine) publishLeave(ch Channel, message *LeaveMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *testEngine) publishAdmin(message *AdminMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *testEngine) publishControl(message *ControlMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *testEngine) subscribe(ch Channel) error {
	return nil
}

func (e *testEngine) unsubscribe(ch Channel) error {
	return nil
}

func (e *testEngine) addPresence(ch Channel, uid ConnID, info ClientInfo) error {
	return nil
}

func (e *testEngine) removePresence(ch Channel, uid ConnID) error {
	return nil
}

func (e *testEngine) presence(ch Channel) (map[ConnID]ClientInfo, error) {
	return map[ConnID]ClientInfo{}, nil
}

func (e *testEngine) history(ch Channel, opts historyOpts) ([]Message, error) {
	return []Message{}, nil
}

func (e *testEngine) channels() ([]Channel, error) {
	return []Channel{}, nil
}
