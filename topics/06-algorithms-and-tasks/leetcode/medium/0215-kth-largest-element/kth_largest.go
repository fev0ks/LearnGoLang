package kth_largest

import "container/heap"

// LeetCode 215. Kth Largest Element in an Array (Medium)
// https://leetcode.com/problems/kth-largest-element-in-an-array/
//
// Задача: найти k-й по величине элемент массива (k-й в порядке убывания, не
// k-й уникальный).
//
//	[3,2,1,5,6,4], k=2 -> 5
//	[3,2,3,1,2,4,5,5,6], k=4 -> 4
//
// Идея: держим min-heap размера k. Проходим по числам, добавляя каждое; если
// размер превысил k, выкидываем минимум. В итоге в куче остаются k наибольших
// элементов, а её корень — это и есть k-й по величине.
//
// Сложность: O(n log k) по времени, O(k) по памяти.
func FindKthLargest(nums []int, k int) int {
	h := &minHeap{}
	for _, x := range nums {
		heap.Push(h, x)
		if h.Len() > k {
			heap.Pop(h) // удаляем минимум — он не входит в k наибольших
		}
	}
	return (*h)[0]
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
