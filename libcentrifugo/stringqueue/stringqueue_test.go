package stringqueue

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringQueueResize(t *testing.T) {
	q := New()
	assert.Equal(t, 0, q.Len())
	assert.Equal(t, initialCapacity, q.Cap())
	assert.Equal(t, false, q.Closed())

	i := 0
	for i < initialCapacity {
		q.Add(strconv.Itoa(i))
		i++
	}
	assert.Equal(t, initialCapacity, q.Cap())
	q.Add("resize here")
	assert.Equal(t, initialCapacity*2, q.Cap())
	q.Remove()
	// back to initial capacity
	assert.Equal(t, initialCapacity, q.Cap())

	q.Add("new resize here")
	assert.Equal(t, initialCapacity*2, q.Cap())
	q.Add("one more item, no resize must happen")
	assert.Equal(t, initialCapacity*2, q.Cap())

	assert.Equal(t, initialCapacity+2, q.Len())
}

func TestStringQueueWait(t *testing.T) {
	q := New()
	q.Add("1")
	q.Add("2")

	s, ok := q.Wait()
	assert.Equal(t, true, ok)
	assert.Equal(t, "1", s)

	s, ok = q.Wait()
	assert.Equal(t, true, ok)
	assert.Equal(t, "2", s)

	go func() {
		q.Add("3")
	}()

	s, ok = q.Wait()
	assert.Equal(t, true, ok)
	assert.Equal(t, "3", s)

}

func TestStringQueueClose(t *testing.T) {
	q := New()

	// test removing from empty queue
	_, ok := q.Remove()
	assert.Equal(t, false, ok)

	q.Add("1")
	q.Add("2")
	q.Close()

	ok = q.Add("3")
	assert.Equal(t, false, ok)

	_, ok = q.Wait()
	assert.Equal(t, false, ok)

	_, ok = q.Remove()
	assert.Equal(t, false, ok)

	assert.Equal(t, true, q.Closed())

}
