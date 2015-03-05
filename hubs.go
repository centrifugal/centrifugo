package main

import (
	"sync"
)

// hub manages client connections
type hub struct {
	sync.Mutex
	connections map[string]map[string]map[string]*connection
}

func newHub() *hub {
	return &hub{
		connections: make(map[string]map[string]map[string]*connection),
	}
}

func (h *hub) add(c *connection) {
	h.Lock()
	defer h.Unlock()

	uid := c.getUid()
	user := c.getUser()
	project := c.getProject()

	_, ok := h.connections[project]
	if !ok {
		h.connections[project] = make(map[string]map[string]*connection)
	}
	_, ok = h.connections[project][user]
	if !ok {
		h.connections[project][user] = make(map[string]*connection)
	}
	h.connections[project][user][uid] = c
}

func (h *hub) remove(c *connection) {
	h.Lock()
	defer h.Unlock()

	uid := c.getUid()
	user := c.getUser()
	project := c.getProject()

	// try to find connection to delete, return early if not found
	if _, ok := h.connections[project]; !ok {
		return
	}
	if _, ok = h.connections[project][user]; !ok {
		return
	}
	if _, ok = h.connections[project][user][uid]; !ok {
		return
	}

	// actually remove connection from hub
	delete(h.connections[project][user], uid)

	// clean up map if it's needed
	if len(h.connections[project][user]) == 0 {
		delete(h.connections[project], user)
	}
	if len(h.connections[project]) == 0 {
		delete(h.connections, project)
	}

}

// adminHub manages admin connections from web interface
type adminHub struct {
	sync.Mutex
	connections map[string]*connection
}

func newAdminHub() *adminHub {
	return &adminHub{
		connections: make(map[string]*connection),
	}
}

func (h *adminHub) add(c *connection) {
	h.Lock()
	defer h.Unlock()
	h.connections[c.getUid()] = c
}

func (h *adminHub) remove(c *connection) {
	h.Lock()
	defer h.Unlock()
	delete(h.connections, c.getUid())
}
