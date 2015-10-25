package libcentrifugo

import (
	"testing"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

func TestCPUUsage(t *testing.T) {
	_, err := cpuUsage()
	assert.Equal(t, nil, err)
}
