// http://rogpeppe.wordpress.com/2009/12/01/concurrent-idioms-1-broadcasting-values-in-go-with-linked-channels/
package broadcast

type Hub struct {
	// Registered connections.
	connections map[chan struct{}]bool

	// Inbound messages from the connections.
	Broadcast chan struct{}

	// Register requests from the connections.
	Register chan chan struct{}

	// Unregister requests from connections.
	Unregister chan chan struct{}
}

func NewHub() *Hub {
	return &Hub{
		Broadcast:   make(chan struct{}),
		Register:    make(chan chan struct{}),
		Unregister:  make(chan chan struct{}),
		connections: make(map[chan struct{}]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.Register:
			h.connections[c] = true
		case c := <-h.Unregister:
			if _, ok := h.connections[c]; ok {
				delete(h.connections, c)
				close(c)
			}
		case m := <-h.Broadcast:
			for c := range h.connections {
				select {
				case c <- m:
				default:
					delete(h.connections, c)
					close(c)
				}
			}
		}
	}
}
