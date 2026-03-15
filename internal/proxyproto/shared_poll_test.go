package proxyproto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeSharedPollRefreshRequest(t *testing.T) {
	encoder := &JSONEncoder{}
	req := &SharedPollRefreshRequest{
		Channel: "test:channel",
		Items: []*SharedPollRefreshItem{
			{Key: "key1", Version: 0},
			{Key: "key2", Version: 5},
		},
	}
	data, err := encoder.EncodeSharedPollRefreshRequest(req)
	require.NoError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, "test:channel", decoded["channel"])

	items := decoded["items"].([]interface{})
	require.Len(t, items, 2)

	item0 := items[0].(map[string]interface{})
	require.Equal(t, "key1", item0["key"])
	// Version 0 should be omitted (omitempty).
	_, hasVersion := item0["version"]
	require.False(t, hasVersion)

	item1 := items[1].(map[string]interface{})
	require.Equal(t, "key2", item1["key"])
	require.Equal(t, float64(5), item1["version"])
}

func TestEncodeSharedPollRefreshRequest_EmptyItems(t *testing.T) {
	encoder := &JSONEncoder{}
	req := &SharedPollRefreshRequest{
		Channel: "ch",
		Items:   []*SharedPollRefreshItem{},
	}
	data, err := encoder.EncodeSharedPollRefreshRequest(req)
	require.NoError(t, err)
	require.Contains(t, string(data), `"channel":"ch"`)
}

func TestDecodeSharedPollRefreshResponse(t *testing.T) {
	decoder := &JSONDecoder{}
	input := `{
		"result": {
			"items": [
				{"key": "key1", "data": {"votes": 42}, "version": 5},
				{"key": "key2", "data": {"votes": 10}, "version": 3, "removed": true}
			]
		}
	}`
	resp, err := decoder.DecodeSharedPollRefreshResponse([]byte(input))
	require.NoError(t, err)
	require.NotNil(t, resp.Result)
	require.Len(t, resp.Result.Items, 2)

	item0 := resp.Result.Items[0]
	require.Equal(t, "key1", item0.Key)
	require.Equal(t, uint64(5), item0.Version)
	require.False(t, item0.Removed)
	require.Contains(t, string(item0.Data), "42")

	item1 := resp.Result.Items[1]
	require.Equal(t, "key2", item1.Key)
	require.Equal(t, uint64(3), item1.Version)
	require.True(t, item1.Removed)
}

func TestDecodeSharedPollRefreshResponse_EmptyItems(t *testing.T) {
	decoder := &JSONDecoder{}
	input := `{"result": {"items": []}}`
	resp, err := decoder.DecodeSharedPollRefreshResponse([]byte(input))
	require.NoError(t, err)
	require.NotNil(t, resp.Result)
	require.Empty(t, resp.Result.Items)
}

func TestDecodeSharedPollRefreshResponse_MissingFields(t *testing.T) {
	decoder := &JSONDecoder{}
	input := `{"result": {"items": [{"key": "key1"}]}}`
	resp, err := decoder.DecodeSharedPollRefreshResponse([]byte(input))
	require.NoError(t, err)
	require.Len(t, resp.Result.Items, 1)
	item := resp.Result.Items[0]
	require.Equal(t, "key1", item.Key)
	require.Equal(t, uint64(0), item.Version)
	require.False(t, item.Removed)
	require.Nil(t, item.Data)
	require.Nil(t, item.PrevData)
}

func TestDecodeSharedPollRefreshResponse_WithPrevData(t *testing.T) {
	decoder := &JSONDecoder{}
	input := `{"result": {"items": [{"key": "k1", "data": {"v": 1}, "version": 2, "prev_data": {"v": 0}}]}}`
	resp, err := decoder.DecodeSharedPollRefreshResponse([]byte(input))
	require.NoError(t, err)
	require.Len(t, resp.Result.Items, 1)
	require.NotNil(t, resp.Result.Items[0].PrevData)
	require.Contains(t, string(resp.Result.Items[0].PrevData), "0")
}

func TestDecodeSharedPollRefreshResponse_ErrorResponse(t *testing.T) {
	decoder := &JSONDecoder{}
	input := `{"error": {"code": 100, "message": "internal error"}}`
	resp, err := decoder.DecodeSharedPollRefreshResponse([]byte(input))
	require.NoError(t, err)
	require.NotNil(t, resp.Error)
	require.Equal(t, uint32(100), resp.Error.Code)
	require.Equal(t, "internal error", resp.Error.Message)
	require.Nil(t, resp.Result)
}

func TestDecodeSharedPollRefreshResponse_NullResult(t *testing.T) {
	decoder := &JSONDecoder{}
	input := `{"result": null}`
	resp, err := decoder.DecodeSharedPollRefreshResponse([]byte(input))
	require.NoError(t, err)
	require.Nil(t, resp.Result)
}

func TestDecodeSharedPollRefreshResponse_InvalidJSON(t *testing.T) {
	decoder := &JSONDecoder{}
	_, err := decoder.DecodeSharedPollRefreshResponse([]byte(`not json`))
	require.Error(t, err)
}

func TestSharedPollRefreshRequest_RoundTrip(t *testing.T) {
	encoder := &JSONEncoder{}
	decoder := &JSONDecoder{}

	original := &SharedPollRefreshRequest{
		Channel: "test:ch",
		Items: []*SharedPollRefreshItem{
			{Key: "k1", Version: 10},
			{Key: "k2", Version: 0},
			{Key: "k3", Version: 999},
		},
	}

	data, err := encoder.EncodeSharedPollRefreshRequest(original)
	require.NoError(t, err)

	// Wrap in response format to test decode.
	respJSON := `{"result": {"items": [{"key": "k1", "data": {"x": 1}, "version": 11}, {"key": "k3", "data": {"x": 3}, "version": 1000}]}}`
	resp, err := decoder.DecodeSharedPollRefreshResponse([]byte(respJSON))
	require.NoError(t, err)
	require.Len(t, resp.Result.Items, 2)

	// Verify the request was encoded properly.
	var reqDecoded SharedPollRefreshRequest
	err = json.Unmarshal(data, &reqDecoded)
	require.NoError(t, err)
	require.Equal(t, "test:ch", reqDecoded.Channel)
	require.Len(t, reqDecoded.Items, 3)
}

func TestEncodeSharedPollRefreshRequest_DiffMode(t *testing.T) {
	encoder := &JSONEncoder{}
	req := &SharedPollRefreshRequest{
		Channel: "ch",
		Items: []*SharedPollRefreshItem{
			{Key: "k1", Version: 5},
			{Key: "k2", Version: 10},
		},
	}
	data, err := encoder.EncodeSharedPollRefreshRequest(req)
	require.NoError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	items := decoded["items"].([]interface{})
	item0 := items[0].(map[string]interface{})
	require.Equal(t, float64(5), item0["version"])
	item1 := items[1].(map[string]interface{})
	require.Equal(t, float64(10), item1["version"])
}

func TestDecodeSharedPollRefreshResponse_RemovedItem(t *testing.T) {
	decoder := &JSONDecoder{}
	input := `{"result": {"items": [{"key": "k1", "removed": true}]}}`
	resp, err := decoder.DecodeSharedPollRefreshResponse([]byte(input))
	require.NoError(t, err)
	require.Len(t, resp.Result.Items, 1)
	require.True(t, resp.Result.Items[0].Removed)
	require.Equal(t, "k1", resp.Result.Items[0].Key)
	require.Equal(t, uint64(0), resp.Result.Items[0].Version)
}
