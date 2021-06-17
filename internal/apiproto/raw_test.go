package apiproto

import (
	"encoding/json"
	"testing"
)

type DataRawMessage struct {
	Data *json.RawMessage
}

type DataRaw struct {
	Data *Raw
}

func TestRaw(t *testing.T) {
	data1 := json.RawMessage(`{"key": "value"}`)
	stdJsonData1, err := json.Marshal(DataRawMessage{
		Data: &data1,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}

	data2 := Raw(`{"key": "value"}`)
	stdJsonData2, err := json.Marshal(DataRaw{
		Data: &data2,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if string(stdJsonData1) != string(stdJsonData2) {
		t.Fatalf("no match: %v", err)
	}
}
