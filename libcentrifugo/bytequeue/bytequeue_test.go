package bytequeue

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestByteQueueResize(t *testing.T) {
	initialCapacity := 2
	q := New(initialCapacity)
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
	initialCapacity := 2
	q := New(initialCapacity)
	assert.Equal(t, 0, q.Size())
	q.Add([]byte("1"))
	q.Add([]byte("2"))
	assert.Equal(t, 2, q.Size())
	q.Remove()
	assert.Equal(t, 1, q.Size())
}

func TestByteQueueWait(t *testing.T) {
	initialCapacity := 2
	q := New(initialCapacity)
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
	initialCapacity := 2
	q := New(initialCapacity)

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

func TestByteQueueCloseRemaining(t *testing.T) {
	q := New(2)
	q.Add([]byte("1"))
	q.Add([]byte("2"))
	msgs := q.CloseRemaining()
	assert.Equal(t, 2, len(msgs))
	ok := q.Add([]byte("3"))
	assert.Equal(t, false, ok)
	assert.Equal(t, true, q.Closed())
	msgs = q.CloseRemaining()
	assert.Equal(t, 0, len(msgs))
}

func BenchmarkQueueAdd(b *testing.B) {
	q := New(2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Add([]byte("test"))
	}
	b.StopTimer()
	q.Close()
}

func addAndConsume(q ByteQueue, n int) {
	// Add to queue and consume in another goroutine.
	done := make(chan struct{})
	go func() {
		count := 0
		for {
			_, ok := q.Wait()
			if !ok {
				continue
			}
			count++
			if count == n {
				close(done)
				break
			}
		}
	}()
	for i := 0; i < n; i++ {
		q.Add([]byte("test"))
	}
	<-done
}

func BenchmarkQueueAddConsume(b *testing.B) {
	q := New(2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		addAndConsume(q, 10000)
	}
	b.StopTimer()
	q.Close()
}
