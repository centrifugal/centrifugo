package libcentrifugo

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAPIResponse(t *testing.T) {
	resp := newAPIPublishResponse()
	marshalledResponse, err := json.Marshal(resp)
	assert.Equal(t, nil, err)

	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":null"))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"body\":null"))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"method\":\"publish\""))
	assert.Equal(t, false, strings.Contains(string(marshalledResponse), "\"uid\""))

	resp = newAPIPublishResponse()
	resp.SetErr(responseError{errors.New("test error"), errorAdviceNone})
	resp.SetUID("test uid")
	marshalledResponse, err = json.Marshal(resp)
	assert.Equal(t, nil, err)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":\"test error\""))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"method\":\"publish\""))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"uid\":\"test uid\""))

	resp = newAPIPublishResponse()
	resp.SetErr(responseError{errors.New("error1"), errorAdviceNone})
	resp.SetErr(responseError{errors.New("error2"), errorAdviceNone})
	marshalledResponse, err = json.Marshal(resp)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":\"error1\""))
}

func TestMultiResponse(t *testing.T) {
	var mr multiAPIResponse
	resp1 := newAPIPublishResponse()
	resp2 := newAPIPublishResponse()
	mr = append(mr, resp1)
	mr = append(mr, resp2)
	marshalledResponse, err := json.Marshal(mr)
	assert.Equal(t, nil, err)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":null"))
}

func TestClientResponse(t *testing.T) {
	resp := newClientPublishResponse(publishBody{Status: true})
	resp.SetErr(responseError{errors.New("error1"), errorAdviceFix})
	resp.SetErr(responseError{errors.New("error2"), errorAdviceFix})
	marshalledResponse, err := json.Marshal(resp)
	assert.Equal(t, nil, err)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":\"error1\""))
}

// TestClientMessageMarshalManual tests valid using of buffer pools
// when marshalling JSON messages manually.
// This is related to https://github.com/centrifugal/centrifugo/issues/94
// and without fix reproduces problem described in issue.
func TestClientMessageMarshalManual(t *testing.T) {
	responses := make([]*clientMessageResponse, 1000)

	payloads := []string{
		`{"input": "test", "id":"TEST.12","value":47.355434343434343,"timestamp":14679931924}`,
		`{"input": "test", "id":"12.TEST","value":37.355,"timestamp":14679936319242121}`,
	}

	for i := 0; i < 1000; i++ {
		resp := newClientMessage()
		resp.Body = *newMessage(Channel("test"), []byte(payloads[i%len(payloads)]), "", nil)
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
		var m clientMessageResponse
		err := json.Unmarshal(resp, &m)
		if err != nil {
			t.Fatal(err)
		}
	}
}
