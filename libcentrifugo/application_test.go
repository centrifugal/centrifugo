package libcentrifugo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestConfig() *config {
	return &config{
		channelPrefix:            "testPrefix",
		namespaceChannelBoundary: ":",
		privateChannelPrefix:     "$",
		userChannelBoundary:      "#",
		userChannelSeparator:     ",",
	}
}

func testApp() *application {
	app, _ := newApplication()
	app.config = newTestConfig()
	app.setEngine(newTestEngine())
	app.structure = getTestStructure()
	return app
}

func TestProjectByKey(t *testing.T) {
	app := testApp()
	p, found := app.projectByKey("nonexistent")
	assert.Equal(t, found, false)
	p, found = app.projectByKey("test1")
	assert.Equal(t, found, true)
	assert.Equal(t, p.Name, ProjectKey("test1"))
}

func TestChannelID(t *testing.T) {
	app := testApp()
	p, _ := app.projectByKey("test1")
	chID := app.channelID(p.Name, "channel")
	assert.Equal(t, chID, ChannelID("testPrefix.test1.channel"))
}

func TestUserAllowed(t *testing.T) {
	app := testApp()
	assert.Equal(t, true, app.userAllowed("channel#1", "1"))
	assert.Equal(t, true, app.userAllowed("channel", "1"))
	assert.Equal(t, false, app.userAllowed("channel#1", "2"))
	assert.Equal(t, true, app.userAllowed("channel#1,2", "1"))
	assert.Equal(t, true, app.userAllowed("channel#1,2", "2"))
	assert.Equal(t, false, app.userAllowed("channel#1,2", "3"))
}
