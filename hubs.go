package main

import (
	"sync"
)

// hub manages client connections
type hub struct {
	sync.Mutex

	// registry to hold active connections
	// as map[of projects]map[of user IDs]map[unique connection IDs]
	connections map[string]map[string]map[string]connection
}

// newHub initializes hub
func newHub() *hub {
	return &hub{
		connections: make(map[string]map[string]map[string]connection),
	}
}

// add adds connection into hub connections registry
func (h *hub) add(c connection) error {
	h.Lock()
	defer h.Unlock()

	uid := c.GetUid()
	user := c.GetUser()
	project := c.GetProject()

	_, ok := h.connections[project]
	if !ok {
		h.connections[project] = make(map[string]map[string]connection)
	}
	_, ok = h.connections[project][user]
	if !ok {
		h.connections[project][user] = make(map[string]connection)
	}
	h.connections[project][user][uid] = c
	return nil
}

// remove removes connection from hub connections registry
func (h *hub) remove(c connection) error {
	h.Lock()
	defer h.Unlock()

	uid := c.GetUid()
	user := c.GetUser()
	project := c.GetProject()

	// try to find connection to delete, return early if not found
	if _, ok := h.connections[project]; !ok {
		return nil
	}
	if _, ok := h.connections[project][user]; !ok {
		return nil
	}
	if _, ok := h.connections[project][user][uid]; !ok {
		return nil
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

	return nil
}

// adminHub manages admin connections from web interface
type adminHub struct {
	sync.Mutex

	// registry to hold active admin connections
	// as map[unique admin connection IDs]
	connections map[string]connection
}

// newAdmin hub initializes new adminHub
func newAdminHub() *adminHub {
	return &adminHub{
		connections: make(map[string]connection),
	}
}

// add adds connection to adminHub connections registry
func (h *adminHub) add(c connection) error {
	h.Lock()
	defer h.Unlock()
	h.connections[c.GetUid()] = c
	return nil
}

// remove removes connection from adminHub connections registry
func (h *adminHub) remove(c connection) error {
	h.Lock()
	defer h.Unlock()
	delete(h.connections, c.GetUid())
	return nil
}
