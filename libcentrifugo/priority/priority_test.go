package priority

import (
	"container/heap"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueue(t *testing.T) {
	pq := MakeQueue()
	assert.Equal(t, pq.Len(), 0)
	heap.Push(&pq, &Item{Value: "2", Priority: 2})
	heap.Push(&pq, &Item{Value: "1", Priority: 1})
	heap.Push(&pq, &Item{Value: "3", Priority: 3})
	assert.Equal(t, pq.Len(), 3)
	item := heap.Pop(&pq).(*Item)
	assert.Equal(t, item.Value, "1")
	item = heap.Pop(&pq).(*Item)
	assert.Equal(t, item.Value, "2")
	item = heap.Pop(&pq).(*Item)
	assert.Equal(t, item.Value, "3")
	assert.Equal(t, pq.Len(), 0)
}
