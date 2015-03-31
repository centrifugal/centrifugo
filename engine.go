package main

type engine interface {
	publishMessage(channel, message string) error
	publishControlMessage(message string) error
	publishAdminMessage(message string) error
	handleMessage(channel, message string) error
	handleControlMessage(message string) error
	handleAdminMessage(message string) error
	addSubscription(channel string, c *connection) error
	removeSubscription(channel string, c *connection) error
	addPresence(channel string, c *connection) error
	removePresence(channel string, c *connection) error
	getPresence(channel string) (map[string]interface{}, error)
	addHistoryMessage(channel string, message string) error
	getHistory(channel string) (map[string]interface{}, error)
}

type memoryEngine struct {
	app *application
}

func newMemoryEngine(app *application) *memoryEngine {
	return &memoryEngine{
		app: app,
	}
}
