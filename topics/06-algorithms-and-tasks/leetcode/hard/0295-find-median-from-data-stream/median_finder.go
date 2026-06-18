package median_finder

import "container/heap"

// LeetCode 295. Find Median from Data Stream (Hard)
// https://leetcode.com/problems/find-median-from-data-stream/
//
// Задача: поддерживать поток чисел и уметь в любой момент за O(log n) добавлять
// число и за O(1) возвращать медиану всех добавленных.
//
//   AddNum(1); AddNum(2); FindMedian() -> 1.5
//   AddNum(3);            FindMedian() -> 2.0
//
// Идея — две кучи: low (max-heap) хранит меньшую половину чисел, high (min-heap)
// — большую. Держим инвариант len(low) >= len(high) и разницу ≤ 1. Тогда
// медиана — это вершина low (нечётное количество) или среднее вершин (чётное).
//
// Сложность: AddNum — O(log n), FindMedian — O(1).

type MedianFinder struct {
	low  *maxHeap // меньшая половина, максимум наверху
	high *minHeap // большая половина, минимум наверху
}

func Constructor() MedianFinder {
	return MedianFinder{low: &maxHeap{}, high: &minHeap{}}
}

func (m *MedianFinder) AddNum(num int) {
	// Кладём в low, затем «перетекаем» максимум low в high, чтобы порядок
	// половин сохранялся.
	heap.Push(m.low, num)
	heap.Push(m.high, heap.Pop(m.low))

	// Восстанавливаем баланс: low должен быть не меньше high.
	if m.high.Len() > m.low.Len() {
		heap.Push(m.low, heap.Pop(m.high))
	}
}

func (m *MedianFinder) FindMedian() float64 {
	if m.low.Len() > m.high.Len() {
		return float64((*m.low)[0])
	}
	return (float64((*m.low)[0]) + float64((*m.high)[0])) / 2
}

// maxHeap — max-heap целых.
type maxHeap []int

func (h maxHeap) Len() int           { return len(h) }
func (h maxHeap) Less(i, j int) bool { return h[i] > h[j] }
func (h maxHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *maxHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *maxHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// minHeap — min-heap целых.
type minHeap []int

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
