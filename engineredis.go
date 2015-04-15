package main

type redisEngine struct {
	app *application
}

func newRedisEngine(app *application) *redisEngine {
	return &redisEngine{
		app: app,
	}
}

func (e *redisEngine) getName() string {
	return "Redis"
}

func (e *redisEngine) publishMessage(channel, message string) error {
	return nil
}

func (e *redisEngine) addSubscription(channel string, c *connection) error {
	return nil
}

func (e *redisEngine) removeSubscription(channel string, c *connection) error {
	return nil
}

func (e *redisEngine) addPresence(channel string, c *connection) error {
	return nil
}

func (e *redisEngine) removePresence(channel string, c *connection) error {
	return nil
}

func (e *redisEngine) getPresence(channel string) (interface{}, error) {
	return map[string]interface{}{}, nil
}

func (e *redisEngine) addHistoryMessage(channel string, message string) error {
	return nil
}

func (e *redisEngine) getHistory(channel string) (interface{}, error) {
	return map[string]interface{}{}, nil
}
