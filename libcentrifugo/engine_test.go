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

func (e *testEngine) publish(chID ChannelID, message []byte, opts *publishOpts) <-chan error {
	ch := make(chan error, 1)
	ch <- nil
	return ch
}

func (e *testEngine) subscribe(chID ChannelID) error {
	return nil
}

func (e *testEngine) unsubscribe(chID ChannelID) error {
	return nil
}

func (e *testEngine) addPresence(chID ChannelID, uid ConnID, info ClientInfo) error {
	return nil
}

func (e *testEngine) removePresence(chID ChannelID, uid ConnID) error {
	return nil
}

func (e *testEngine) presence(chID ChannelID) (map[ConnID]ClientInfo, error) {
	return map[ConnID]ClientInfo{}, nil
}

func (e *testEngine) history(chID ChannelID, opts historyOpts) ([]Message, error) {
	return []Message{}, nil
}

func (e *testEngine) channels() ([]ChannelID, error) {
	return []ChannelID{}, nil
}
