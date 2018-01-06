package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCounter(t *testing.T) {
	cnt := NewCounter()
	assert.Equal(t, int64(0), cnt.IntervalValue())
	for i := 0; i < 1000; i++ {
		cnt.Inc()
	}
	assert.Equal(t, int64(1000), cnt.Value())
	cnt.Add(500)
	assert.Equal(t, int64(1500), cnt.Value())
	cnt.UpdateDelta()
	cnt.Inc()
	assert.Equal(t, int64(1501), cnt.Value())
	assert.Equal(t, int64(1500), cnt.IntervalValue())
}

func TestCounterRegistry(t *testing.T) {
	cnt := NewCounter()
	reg := NewCounterRegistry()
	name := "test_counter"
	reg.Register(name, cnt)
	reg.Inc(name)
	reg.Inc(name)
	assert.Equal(t, int64(2), reg.Get(name).Value())
	reg.Add(name, 3)
	assert.Equal(t, int64(5), reg.Get(name).Value())
	reg.UpdateDelta()
	reg.Inc(name)
	assert.Equal(t, int64(6), reg.Get(name).Value())
	assert.Equal(t, int64(5), reg.Get(name).IntervalValue())
	values := reg.LoadValues()
	val, ok := values[name]
	assert.True(t, ok)
	assert.Equal(t, int64(6), int64(val))
	intervalValues := reg.LoadIntervalValues()
	val, ok = intervalValues[name]
	assert.True(t, ok)
	assert.Equal(t, int64(5), int64(val))

	newName := "test_counter_2"
	reg.Register(newName, NewCounter())
	values = reg.LoadValues(newName)
	assert.Equal(t, 1, len(values))
	values = reg.LoadIntervalValues(newName)
	assert.Equal(t, 1, len(values))
}
