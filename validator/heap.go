package validator

// TransactionHeap satisfies `container/heap` for transactions.
type TransactionHeap []*Transaction

func (heap TransactionHeap) Len() int {
	return len(heap)
}

func (heap TransactionHeap) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, priority so we use greater than here.
	return heap[i].prio > heap[j].prio
}

func (heap TransactionHeap) Swap(i, j int) {
	heap[i], heap[j] = heap[j], heap[i]
	heap[i].index = i
	heap[j].index = j
}

func (heap *TransactionHeap) Push(tx any) {
	n := len(*heap)
	item := tx.(*Transaction)
	item.index = n
	*heap = append(*heap, item)
}

func (heap *TransactionHeap) Pop() any {
	old := *heap
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // don't stop the GC from reclaiming the item eventually
	item.index = -1 // for safety
	*heap = old[0 : n-1]
	return item
}
