package messagefilter

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/centrifugal/centrifuge"
)

// MessageInterceptor intercepts and filters messages before they are sent to clients
type MessageInterceptor struct {
	FilterManager *FilterManager
	mu            sync.RWMutex
	clientFilters map[string]map[string]interface{} // clientID -> channel -> filter
}

// NewMessageInterceptor creates a new message interceptor
func NewMessageInterceptor(filterManager *FilterManager) *MessageInterceptor {
	return &MessageInterceptor{
		FilterManager: filterManager,
		clientFilters: make(map[string]map[string]interface{}),
	}
}

// AddClientFilter adds a filter for a specific client and channel
func (mi *MessageInterceptor) AddClientFilter(clientID, channel string, filter interface{}) {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	if mi.clientFilters[clientID] == nil {
		mi.clientFilters[clientID] = make(map[string]interface{})
	}
	mi.clientFilters[clientID][channel] = filter
}

// RemoveClientFilter removes a filter for a specific client and channel
func (mi *MessageInterceptor) RemoveClientFilter(clientID, channel string) {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	if clientFilters, exists := mi.clientFilters[clientID]; exists {
		delete(clientFilters, channel)
		if len(clientFilters) == 0 {
			delete(mi.clientFilters, clientID)
		}
	}
}

// RemoveClient removes all filters for a specific client
func (mi *MessageInterceptor) RemoveClient(clientID string) {
	mi.mu.Lock()
	defer mi.mu.Unlock()
	delete(mi.clientFilters, clientID)
}

// ShouldDeliverMessage checks if a message should be delivered to a specific client
func (mi *MessageInterceptor) ShouldDeliverMessage(ctx context.Context, clientID, channel string, pub *centrifuge.Publication) (bool, error) {
	mi.mu.RLock()
	clientFilters, exists := mi.clientFilters[clientID]
	mi.mu.RUnlock()

	if !exists {
		return true, nil // No filters for this client
	}

	// Use publication meta for filtering
	meta := pub.Meta

	// Check for exact channel match
	filterInterface, exists := clientFilters[channel]
	if exists && filterInterface != nil {
		// Handle filter evaluation
		if filter, ok := filterInterface.(*Filter); ok {
			// Convert meta map to JSON bytes for evaluation
			var metaBytes []byte
			if meta != nil {
				if metaJSON, err := json.Marshal(meta); err == nil {
					metaBytes = metaJSON
				}
			}

			shouldDeliver, err := filter.Evaluate(ctx, metaBytes)
			if err != nil {
				return true, nil // Allow message on error
			}

			return shouldDeliver, nil
		}

		return true, nil // Unknown filter type, allow message
	}

	// No filters found for this channel
	return true, nil
}

// GetStats returns interceptor statistics
func (mi *MessageInterceptor) GetStats() map[string]interface{} {
	mi.mu.RLock()
	defer mi.mu.RUnlock()

	totalClients := len(mi.clientFilters)
	totalFilters := 0

	for _, clientFilters := range mi.clientFilters {
		totalFilters += len(clientFilters)
	}

	// Get filter manager stats
	filterManagerStats := mi.FilterManager.GetStats()

	return map[string]interface{}{
		"total_clients":  totalClients,
		"total_filters":  totalFilters,
		"filter_manager": filterManagerStats,
	}
}
