package libcentrifugo

import ()

func getTestChannelOptions() ChannelOptions {
	return ChannelOptions{
		Watch:           true,
		Publish:         true,
		Presence:        true,
		HistorySize:     1,
		HistoryLifetime: 1,
	}
}

func getTestNamespace(name NamespaceKey) Namespace {
	return Namespace{
		Name:           name,
		ChannelOptions: getTestChannelOptions(),
	}
}
