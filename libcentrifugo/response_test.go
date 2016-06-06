package libcentrifugo

import (
	"encoding/json"
	"errors"
	"strings"
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
