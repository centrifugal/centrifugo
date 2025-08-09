package api

import (
	"context"
	"encoding/json"

	. "github.com/centrifugal/centrifugo/v6/internal/apiproto"
	"github.com/centrifugal/centrifugo/v6/internal/messagefilter"
	"github.com/rs/zerolog/log"
)

// MessageFilterAPI provides API for message filtering functionality
type MessageFilterAPI struct {
	Interceptor *messagefilter.MessageInterceptor
}

// NewMessageFilterAPI creates a new message filter API
func NewMessageFilterAPI(interceptor *messagefilter.MessageInterceptor) *MessageFilterAPI {
	return &MessageFilterAPI{
		Interceptor: interceptor,
	}
}

// FilterStatsRequest represents a request for filter statistics
type FilterStatsRequest struct {
	// No fields needed for now
}

// FilterStatsResponse represents a response with filter statistics
type FilterStatsResponse struct {
	Stats map[string]interface{} `json:"stats"`
}

// TestFilterRequest represents a request to test a CEL filter expression
type TestFilterRequest struct {
	Expression string          `json:"expression"`
	Message    json.RawMessage `json:"message"`
}

// TestFilterResponse represents a response from testing a filter
type TestFilterResponse struct {
	Result bool   `json:"result"`
	Error  string `json:"error,omitempty"`
}

// HandleFilterStats handles the filter stats RPC call
func (api *MessageFilterAPI) HandleFilterStats(ctx context.Context, params Raw) (Raw, error) {
	stats := api.Interceptor.GetStats()
	
	response := FilterStatsResponse{
		Stats: stats,
	}
	
	result, err := json.Marshal(response)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal filter stats response")
		return nil, err
	}
	
	return result, nil
}

// HandleTestFilter handles the test filter RPC call
func (api *MessageFilterAPI) HandleTestFilter(ctx context.Context, params Raw) (Raw, error) {
	var req TestFilterRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	
	if req.Expression == "" {
		response := TestFilterResponse{
			Error: "expression is required",
		}
		result, _ := json.Marshal(response)
		return result, nil
	}
	
	// Create a temporary filter to test the expression
	filter, err := api.Interceptor.FilterManager.GetOrCreateFilter(req.Expression)
	if err != nil {
		response := TestFilterResponse{
			Error: err.Error(),
		}
		result, _ := json.Marshal(response)
		return result, nil
	}
	
	// Test the filter with the provided message
	result, err := filter.Evaluate(ctx, req.Message)
	if err != nil {
		response := TestFilterResponse{
			Error: err.Error(),
		}
		resultBytes, _ := json.Marshal(response)
		return resultBytes, nil
	}
	
	response := TestFilterResponse{
		Result: result,
	}
	
	resultBytes, err := json.Marshal(response)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal test filter response")
		return nil, err
	}
	
	return resultBytes, nil
} 