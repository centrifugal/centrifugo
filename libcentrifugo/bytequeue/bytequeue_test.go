package bytequeue

import (
	"strconv"
	"testing"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

func TestByteQueueResize(t *testing.T) {
	q := New()
	assert.Equal(t, 0, q.Len())
	assert.Equal(t, initialCapacity, q.Cap())
	assert.Equal(t, false, q.Closed())

	i := 0
	for i < initialCapacity {
		q.Add([]byte(strconv.Itoa(i)))
		i++
	}
	assert.Equal(t, initialCapacity, q.Cap())
	q.Add([]byte("resize here"))
	assert.Equal(t, initialCapacity*2, q.Cap())
	q.Remove()
	// back to initial capacity
	assert.Equal(t, initialCapacity, q.Cap())

	q.Add([]byte("new resize here"))
	assert.Equal(t, initialCapacity*2, q.Cap())
	q.Add([]byte("one more item, no resize must happen"))
	assert.Equal(t, initialCapacity*2, q.Cap())

	assert.Equal(t, initialCapacity+2, q.Len())
}

func TestByteQueueSize(t *testing.T) {
	q := New()
	assert.Equal(t, 0, q.Size())
	q.Add([]byte("1"))
	q.Add([]byte("2"))
	assert.Equal(t, 2, q.Size())
	q.Remove()
	assert.Equal(t, 1, q.Size())
}

func TestByteQueueWait(t *testing.T) {
	q := New()
	q.Add([]byte("1"))
	q.Add([]byte("2"))

	s, ok := q.Wait()
	assert.Equal(t, true, ok)
	assert.Equal(t, "1", string(s))

	s, ok = q.Wait()
	assert.Equal(t, true, ok)
	assert.Equal(t, "2", string(s))

	go func() {
		q.Add([]byte("3"))
	}()

	s, ok = q.Wait()
	assert.Equal(t, true, ok)
	assert.Equal(t, "3", string(s))

}

func TestByteQueueClose(t *testing.T) {
	q := New()

	// test removing from empty queue
	_, ok := q.Remove()
	assert.Equal(t, false, ok)

	q.Add([]byte("1"))
	q.Add([]byte("2"))
	q.Close()

	ok = q.Add([]byte("3"))
	assert.Equal(t, false, ok)

	_, ok = q.Wait()
	assert.Equal(t, false, ok)

	_, ok = q.Remove()
	assert.Equal(t, false, ok)

	assert.Equal(t, true, q.Closed())

}

func BenchmarkQueueAdd(b *testing.B) {
	q := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Add([]byte("test"))
	}
	b.StopTimer()
	q.Close()
}
