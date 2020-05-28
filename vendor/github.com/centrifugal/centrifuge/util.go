package centrifuge

import (
	"bytes"
	"sync"
)

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

var bufferPool = sync.Pool{
	// New is called when a new instance is needed
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func getBuffer() *bytes.Buffer {
	return bufferPool.Get().(*bytes.Buffer)
}

func putBuffer(buf *bytes.Buffer) {
	buf.Reset()
	bufferPool.Put(buf)
}
