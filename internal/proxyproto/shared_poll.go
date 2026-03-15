package proxyproto

// SharedPollRefreshItem represents a single item in a shared poll refresh request.
type SharedPollRefreshItem struct {
	Key     string `json:"key"`
	Version uint64 `json:"version,omitempty"`
}

// SharedPollRefreshRequest is a request to refresh shared poll items.
type SharedPollRefreshRequest struct {
	Channel string                   `json:"channel"`
	Items   []*SharedPollRefreshItem `json:"items"`
}

// SharedPollRefreshResultItem represents a single item in a shared poll refresh response.
type SharedPollRefreshResultItem struct {
	Key      string `json:"key"`
	Data     Raw    `json:"data,omitempty"`
	Version  uint64 `json:"version,omitempty"`
	Removed  bool   `json:"removed,omitempty"`
	PrevData Raw    `json:"prev_data,omitempty"`
}

// SharedPollRefreshResult contains the result of a shared poll refresh.
type SharedPollRefreshResult struct {
	Items []*SharedPollRefreshResultItem `json:"items"`
}

// SharedPollRefreshResponse is a response from the shared poll refresh proxy.
type SharedPollRefreshResponse struct {
	Result *SharedPollRefreshResult `json:"result"`
	Error  *Error                   `json:"error,omitempty"`
}
