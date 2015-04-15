package main

type connectCommand struct {
	Project   string
	User      string
	Timestamp string
	Info      string
	Token     string
}

type refreshCommand struct {
	Project   string
	User      string
	Timestamp string
	Info      string
	Token     string
}

type subscribeCommand struct {
	Channel string
	Client  string
	Info    string
	Sign    string
}

type unsubscribeCommand struct {
	Channel string
}

type publishCommand struct {
	Channel string
	Data    interface{}
}

type presenceCommand struct {
	Channel string
}

type historyCommand struct {
	Channel string
}
