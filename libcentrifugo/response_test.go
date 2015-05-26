package libcentrifugo

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResponse(t *testing.T) {
	resp := newResponse("test")
	marshalledResponse, err := json.Marshal(resp)
	assert.Equal(t, nil, err)

	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":null"))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"body\":null"))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"method\":\"test\""))

	resp = newResponse("test")
	resp.Err(errors.New("test error"))
	resp.Body = "test body"
	marshalledResponse, err = json.Marshal(resp)
	t.Log(string(marshalledResponse))
	assert.Equal(t, nil, err)
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"error\":\"test error\""))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"body\":\"test body\""))
	assert.Equal(t, true, strings.Contains(string(marshalledResponse), "\"method\":\"test\""))
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
