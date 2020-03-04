package centrifuge

import (
	"bytes"
	"math"
	"sync"
)

const (
	maxSeq uint32 = math.MaxUint32 // maximum uint32 value
)

func nextSeqGen(currentSeq, currentGen uint32) (uint32, uint32) {
	var nextSeq uint32
	nextGen := currentGen
	if currentSeq == maxSeq {
		nextSeq = 0
		nextGen++
	} else {
		nextSeq = currentSeq + 1
	}
	return nextSeq, nextGen
}

func unpackUint64(val uint64) (uint32, uint32) {
	return uint32(val), uint32(val >> 32)
}

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
