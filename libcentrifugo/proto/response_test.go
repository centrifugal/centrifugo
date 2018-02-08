package proto

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
	"github.com/stretchr/testify/assert"
)

func TestAPIResponse(t *testing.T) {
	resp := NewAPIPublishResponse()
	marshalledResponse, err := json.Marshal(resp)
	assert.Equal(t, nil, err)

	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":null"))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"body\":null"))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"method\":\"publish\""))
	assert.Equal(t, false, strings.Contains(string(marshalledResponse), "\"uid\""))

	resp = NewAPIPublishResponse()
	resp.SetErr(ResponseError{errors.New("test error"), ErrorAdviceNone})
	resp.SetUID("test uid")
	marshalledResponse, err = json.Marshal(resp)
	assert.Equal(t, nil, err)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":\"test error\""))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"method\":\"publish\""))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"uid\":\"test uid\""))

	resp = NewAPIPublishResponse()
	resp.SetErr(ResponseError{errors.New("error1"), ErrorAdviceNone})
	resp.SetErr(ResponseError{errors.New("error2"), ErrorAdviceNone})
	marshalledResponse, err = json.Marshal(resp)
	assert.Equal(t, nil, err)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":\"error1\""))
}

func TestMultiResponse(t *testing.T) {
	var mr MultiAPIResponse
	resp1 := NewAPIPublishResponse()
	resp2 := NewAPIPublishResponse()
	mr = append(mr, resp1)
	mr = append(mr, resp2)
	marshalledResponse, err := json.Marshal(mr)
	assert.Equal(t, nil, err)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":null"))
}

func TestClientResponse(t *testing.T) {
	resp := NewClientPublishResponse(PublishBody{Status: true})
	resp.SetErr(ResponseError{errors.New("error1"), ErrorAdviceFix})
	resp.SetErr(ResponseError{errors.New("error2"), ErrorAdviceFix})
	marshalledResponse, err := json.Marshal(resp)
	assert.Equal(t, nil, err)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":\"error1\""))
}

func TestAdminMessageResponse(t *testing.T) {
	data := raw.Raw([]byte("test"))
	resp := NewAdminMessageResponse(data)
	assert.Equal(t, "message", resp.Method)
}

// TestClientMessageMarshalManual tests valid using of buffer pools
// when marshalling JSON messages manually.
// This is related to https://github.com/centrifugal/centrifugo/issues/94
// and without fix reproduces problem described in issue.
func TestClientMessageMarshalManual(t *testing.T) {
	responses := make([]*ClientMessageResponse, 1000)

	payloads := []string{
		`{"input": "test", "id":"TEST.12","value":47.355434343434343,"timestamp":14679931924}`,
		`{"input": "test", "id":"12.TEST","value":37.355,"timestamp":14679936319242121}`,
	}

	for i := 0; i < 1000; i++ {
		resp := NewClientMessage(NewMessage("test", []byte(payloads[i%len(payloads)]), "", nil))
		responses[i] = resp
	}

	resps := map[int][]byte{}
	var mapLock sync.Mutex

	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()
			resp, err := responses[num%1000].Marshal()
			if err != nil {
				panic(err)
			}
			mapLock.Lock()
			resps[i] = resp
			mapLock.Unlock()
		}(i)
	}
	wg.Wait()

	for _, resp := range resps {
		var m ClientMessageResponse
		err := json.Unmarshal(resp, &m)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestClientMessageMarshalManualWithClientInfo(t *testing.T) {

	defaultInfo := raw.Raw(`{"default": "info"}`)
	channelInfo := raw.Raw(`{"channel": "info"}`)

	info := &ClientInfo{
		DefaultInfo: defaultInfo,
		ChannelInfo: channelInfo,
		User:        "test_user",
		Client:      "test_client",
	}

	payload := `{"input": "test"}`
	resp := NewClientMessage(NewMessage("test", []byte(payload), "test_client", info))
	jsonData, err := resp.Marshal()
	assert.Equal(t, nil, err)
	assert.True(t, strings.Contains(string(jsonData), `"default_info":{"default": "info"}`))
	assert.True(t, strings.Contains(string(jsonData), `"channel_info":{"channel": "info"}`))
	assert.True(t, strings.Contains(string(jsonData), "test_user"))
	assert.True(t, strings.Contains(string(jsonData), "test_client"))
}
