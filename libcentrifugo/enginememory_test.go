package libcentrifugo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func testMemoryEngine() *MemoryEngine {
	app, _ := NewApplication(newTestConfig())
	e := NewMemoryEngine(app)
	app.SetEngine(e)
	return e
}

func TestMemoryEngine(t *testing.T) {
	e := testMemoryEngine()
	assert.NotEqual(t, nil, e.historyHub)
	assert.NotEqual(t, nil, e.presenceHub)
	assert.NotEqual(t, e.name(), "")
	assert.Equal(t, nil, e.publish(ChannelID("channel"), []byte("{}")))
	assert.Equal(t, nil, e.subscribe(ChannelID("channel")))
	assert.Equal(t, nil, e.unsubscribe(ChannelID("channel")))
}
