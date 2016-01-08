package libcentrifugo

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

func TestResponse(t *testing.T) {
	resp := newResponse("test")
	marshalledResponse, err := json.Marshal(resp)
	assert.Equal(t, nil, err)

	resp.Err(nil)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":null"))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"body\":null"))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"method\":\"test\""))
	assert.Equal(t, false, strings.Contains(string(marshalledResponse), "\"id\""))

	resp = newResponse("test")
	resp.Err(errors.New("test error"))
	resp.Body = "test body"
	resp.UID = "test uid"
	marshalledResponse, err = json.Marshal(resp)
	t.Log(string(marshalledResponse))
	assert.Equal(t, nil, err)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":\"test error\""))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"body\":\"test body\""))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"method\":\"test\""))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"uid\":\"test uid\""))

	resp = newResponse("test")
	resp.Err(errors.New("error1"))
	resp.Err(errors.New("error2"))
	marshalledResponse, err = json.Marshal(resp)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":\"error1\""))
}

func TestMultiResponse(t *testing.T) {
	var mr multiResponse
	resp1 := newResponse("test1")
	resp2 := newResponse("test2")
	mr = append(mr, resp1)
	mr = append(mr, resp2)
	marshalledResponse, err := json.Marshal(mr)
	t.Log(string(marshalledResponse))
	assert.Equal(t, nil, err)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":null"))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"method\":\"test1\""))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"method\":\"test2\""))
}

func TestClientResponse(t *testing.T) {
	resp := newClientResponse("test")
	resp.Err(clientError{errors.New("error1"), errorAdviceFix})
	resp.Err(clientError{errors.New("error2"), errorAdviceFix})
	marshalledResponse, err := json.Marshal(resp)
	assert.Equal(t, nil, err)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":\"error1\""))
}
