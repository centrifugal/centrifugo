package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGauge(t *testing.T) {
	g := NewGauge()
	assert.Equal(t, int64(0), g.Load())
	g.Set(20)
	assert.Equal(t, int64(20), g.Load())
}

func TestGaugeRegistry(t *testing.T) {
	g := NewGauge()
	reg := NewGaugeRegistry()
	name := "test_gauge"
	reg.Register(name, g)
	reg.Set(name, 30)
	assert.Equal(t, int64(30), reg.Get(name).Load())
	values := reg.LoadValues()
	val, ok := values[name]
	assert.True(t, ok)
	assert.Equal(t, int64(30), int64(val))

	newName := "test_gauge_2"
	reg.Register(newName, NewGauge())
	values = reg.LoadValues(newName)
	assert.Equal(t, 1, len(values))
}
