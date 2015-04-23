package main

type redisEngine struct {
	app      *application
	host     string
	port     string
	password string
	db       string
	url      string
	api      bool
}

func newRedisEngine(app *application, host, port, password, db, url string, api bool) *redisEngine {
	return &redisEngine{
		app:      app,
		host:     host,
		port:     port,
		password: password,
		db:       db,
		url:      url,
		api:      api,
	}
}

func (e *redisEngine) getName() string {
	return "Redis"
}

func (e *redisEngine) publish(channel, message string) error {
	return nil
}

func (e *redisEngine) subscribe(channel string) error {
	return nil
}

func (e *redisEngine) unsubscribe(channel string) error {
	return nil
}

func (e *redisEngine) addPresence(channel, uid string, info interface{}) error {
	return nil
}

func (e *redisEngine) removePresence(channel, uid string) error {
	return nil
}

func (e *redisEngine) getPresence(channel string) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (e *redisEngine) addHistoryMessage(channel string, message interface{}) error {
	return nil
}

func (e *redisEngine) getHistory(channel string) ([]interface{}, error) {
	return []interface{}{}, nil
}
