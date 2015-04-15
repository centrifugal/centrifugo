package main

type memoryEngine struct {
	app *application
}

func newMemoryEngine(app *application) *memoryEngine {
	return &memoryEngine{
		app: app,
	}
}

func (e *memoryEngine) getName() string {
	return "In memory – single node only"
}

func (e *memoryEngine) publishMessage(channel, message string) error {
	return nil
}

func (e *memoryEngine) addSubscription(channel string, c *connection) error {
	return nil
}

func (e *memoryEngine) removeSubscription(channel string, c *connection) error {
	return nil
}

func (e *memoryEngine) addPresence(channel string, c *connection) error {
	return nil
}

func (e *memoryEngine) removePresence(channel string, c *connection) error {
	return nil
}

func (e *memoryEngine) getPresence(channel string) (interface{}, error) {
	return map[string]interface{}{}, nil
}

func (e *memoryEngine) addHistoryMessage(channel string, message string) error {
	return nil
}

func (e *memoryEngine) getHistory(channel string) (interface{}, error) {
	return map[string]interface{}{}, nil
}
