package libcentrifugo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

func TestCPUUsage(t *testing.T) {
	_, err := cpuUsage()
	assert.Equal(t, nil, err)
}

func AssertSameCounts(t *testing.T, when string, got, expected metricCounter) {
	if got.value != expected.value {
		t.Errorf("%s, raw counter value doesn't match: got %d expected %d", when, got.value, expected.value)
	}
	if got.lastIntervalValue != expected.lastIntervalValue {
		t.Errorf("%s, counter list interval value doesn't match: got %d expected %d", when,
			got.lastIntervalValue, expected.lastIntervalValue)
	}
	if got.lastIntervalDelta != expected.lastIntervalDelta {
		t.Errorf("%s, counter last interval delta doesn't match: got %d expected %d", when,
			got.lastIntervalDelta, expected.lastIntervalDelta)
	}
}

func TestMetrics(t *testing.T) {

	m := metricsRegistry{}

	m.NumMsgPublished.Inc()
	m.NumMsgPublished.Inc()

	m.NumClientRequests.Add(10)

	// Deltas should all be zero as we didn't update yet
	AssertSameCounts(t, "Before update", m.NumMsgPublished, metricCounter{2, 0, 0})
	AssertSameCounts(t, "Before update", m.NumClientRequests, metricCounter{10, 0, 0})

	// Now update
	m.UpdateSnapshot()

	AssertSameCounts(t, "After update", m.NumMsgPublished, metricCounter{2, 2, 2})
	AssertSameCounts(t, "After update", m.NumClientRequests, metricCounter{10, 10, 10})

	// More increments
	m.NumMsgPublished.Inc()
	m.NumClientRequests.Inc()

	AssertSameCounts(t, "After second update", m.NumMsgPublished, metricCounter{3, 2, 2})
	AssertSameCounts(t, "After second update", m.NumClientRequests, metricCounter{11, 10, 10})

	// Second update
	m.UpdateSnapshot()

	AssertSameCounts(t, "After second update", m.NumMsgPublished, metricCounter{3, 3, 1})
	AssertSameCounts(t, "After second update", m.NumClientRequests, metricCounter{11, 11, 1})
}

func TestJSONMarshal(t *testing.T) {
	m := &metricsRegistry{}
	m.NumMsgPublished.Add(42)
	m.NumMsgQueued.Add(42)
	m.NumMsgSent.Add(42)
	m.NumAPIRequests.Add(42)
	m.NumClientRequests.Add(42)
	m.BytesClientIn.Add(42)
	m.BytesClientOut.Add(42)

	m.UpdateSnapshot()

	expected := fmt.Sprintf(`{"num_msg_published":42,`+
		`"num_msg_queued":42,`+
		`"num_msg_sent":42,`+
		`"num_api_requests":42,`+
		`"num_client_requests":42,`+
		`"bytes_client_in":42,`+
		`"bytes_client_out":42,`+
		// Timers are deprecated but still in output to avoid breaking assumptions
		`"time_api_mean":0,`+
		`"time_client_mean":0,`+
		`"time_api_max":0,`+
		`"time_client_max":0,`+
		`"memory_sys":%d,`+
		`"cpu_usage":%d}`, m.MemSys.value, m.CPU.value)

	jsonBytes, err := json.Marshal(m.GetRawMetrics())
	if err != nil {
		t.Fatalf("JSON Marshal failed: ", err)
	}
	if !bytes.Equal(jsonBytes, []byte(expected)) {
		t.Errorf("JSON Marshal returned:\n\t%s\n  expected:\n\t%s", jsonBytes, expected)
	}

	// Update some values
	m.NumMsgSent.Add(42)
	m.NumAPIRequests.Add(42)
	m.NumClientRequests.Add(42)
	m.BytesClientIn.Add(42)

	// Now snapshot should be just the same since we've not updated
	jsonBytes, err = json.Marshal(m.GetSnapshotMetrics())
	if err != nil {
		t.Fatalf("JSON Marshal failed: ", err)
	}
	if !bytes.Equal(jsonBytes, []byte(expected)) {
		t.Errorf("After incrementing JSON Marshal returned:\n\t%s\n  expected:\n\t%s", jsonBytes, expected)
	}

	// But Raw snapshot should include raw totals
	raw := m.GetRawMetrics()
	expectedRaw := fmt.Sprintf(`{"num_msg_published":42,`+
		`"num_msg_queued":42,`+
		`"num_msg_sent":84,`+
		`"num_api_requests":84,`+
		`"num_client_requests":84,`+
		`"bytes_client_in":84,`+
		`"bytes_client_out":42,`+
		// Timers are deprecated but still in output to avoid breaking assumptions
		`"time_api_mean":0,`+
		`"time_client_mean":0,`+
		`"time_api_max":0,`+
		`"time_client_max":0,`+
		`"memory_sys":%d,`+
		`"cpu_usage":%d}`, raw.MemSys, raw.CPU)

	rawJsonBytes, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("JSON Marshal failed: ", err)
	}
	if !bytes.Equal(rawJsonBytes, []byte(expectedRaw)) {
		t.Errorf("After incrementing raw count JSON Marshal returned:\n\t%s\n  expected:\n\t%s",
			rawJsonBytes, expectedRaw)
	}

	// Now update
	m.UpdateSnapshot()

	// Now snapshot should have reset to only last round of increments
	expected = fmt.Sprintf(`{"num_msg_published":0,`+
		`"num_msg_queued":0,`+
		`"num_msg_sent":42,`+
		`"num_api_requests":42,`+
		`"num_client_requests":42,`+
		`"bytes_client_in":42,`+
		`"bytes_client_out":0,`+
		// Timers are deprecated but still in output to avoid breaking assumptions
		`"time_api_mean":0,`+
		`"time_client_mean":0,`+
		`"time_api_max":0,`+
		`"time_client_max":0,`+
		`"memory_sys":%d,`+
		`"cpu_usage":%d}`, m.MemSys.value, m.CPU.value)

	jsonBytes, err = json.Marshal(m.GetSnapshotMetrics())
	if err != nil {
		t.Fatalf("JSON Marshal failed: ", err)
	}
	if !bytes.Equal(jsonBytes, []byte(expected)) {
		t.Errorf("After second update JSON Marshal returned:\n\t%s\n  expected:\n\t%s", jsonBytes, expected)
	}

	// But Raw should still have all the totals (need to redefine it though since cpu and mem might change
	// during UpdateSnapshot above)
	raw = m.GetRawMetrics()
	expectedRaw = fmt.Sprintf(`{"num_msg_published":42,`+
		`"num_msg_queued":42,`+
		`"num_msg_sent":84,`+
		`"num_api_requests":84,`+
		`"num_client_requests":84,`+
		`"bytes_client_in":84,`+
		`"bytes_client_out":42,`+
		// Timers are deprecated but still in output to avoid breaking assumptions
		`"time_api_mean":0,`+
		`"time_client_mean":0,`+
		`"time_api_max":0,`+
		`"time_client_max":0,`+
		`"memory_sys":%d,`+
		`"cpu_usage":%d}`, raw.MemSys, raw.CPU)
	rawJsonBytes, err = json.Marshal(raw)
	if err != nil {
		t.Fatalf("JSON Marshal failed: ", err)
	}
	if !bytes.Equal(rawJsonBytes, []byte(expectedRaw)) {
		t.Errorf("After second update raw count JSON Marshal returned:\n\t%s\n  expected:\n\t%s", rawJsonBytes, expectedRaw)
	}
}
