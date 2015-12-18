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

func (e *testEngine) publish(chID ChannelID, message []byte) error {
	return nil
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

func (e *testEngine) addHistory(chID ChannelID, message Message, opts historyOptions) error {
	return nil
}

func (e *testEngine) history(chID ChannelID, limit int64) ([]Message, error) {
	return []Message{}, nil
}

func (e *testEngine) channels() ([]ChannelID, error) {
	return []ChannelID{}, nil
}
