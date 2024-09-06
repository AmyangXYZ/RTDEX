package utils

import (
	"container/heap"
	"sync"
	"time"
)

type ExpiringItem struct {
	Value    interface{}
	ExpireAt time.Time
}

type expirationHeap []*ExpiringItem

func (h expirationHeap) Len() int           { return len(h) }
func (h expirationHeap) Less(i, j int) bool { return h[i].ExpireAt.Before(h[j].ExpireAt) }
func (h expirationHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *expirationHeap) Push(x interface{}) {
	*h = append(*h, x.(*ExpiringItem))
}
func (h *expirationHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

type ExpiryPriorityQueue struct {
	items expirationHeap
	mu    sync.Mutex
}

func NewExpiryPriorityQueue() *ExpiryPriorityQueue {
	pq := &ExpiryPriorityQueue{
		items: expirationHeap{},
	}
	heap.Init(&pq.items)
	return pq
}

func (pq *ExpiryPriorityQueue) Push(item *ExpiringItem) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	heap.Push(&pq.items, item)
}

func (pq *ExpiryPriorityQueue) Pop() *ExpiringItem {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return heap.Pop(&pq.items).(*ExpiringItem)
}

func (pq *ExpiryPriorityQueue) Peek() *ExpiringItem {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if len(pq.items) == 0 {
		return nil
	}
	return pq.items[0]
}

func (pq *ExpiryPriorityQueue) Remove(x interface{}) *ExpiringItem {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	for i, item := range pq.items {
		if item.Value == x {
			heap.Remove(&pq.items, i)
			return item
		}
	}
	return nil
}

func (pq *ExpiryPriorityQueue) UpdateExpiration(x interface{}, newExpiration time.Time) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	for i, item := range pq.items {
		if item.Value == x {
			pq.items[i].ExpireAt = newExpiration
			heap.Fix(&pq.items, i)
			return true
		}
	}
	heap.Push(&pq.items, &ExpiringItem{
		Value:    x,
		ExpireAt: newExpiration,
	})
	return true
}
