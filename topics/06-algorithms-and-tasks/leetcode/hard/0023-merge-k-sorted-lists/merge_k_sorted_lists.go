package merge_k_sorted_lists

import "container/heap"

// LeetCode 23. Merge k Sorted Lists (Hard)
// https://leetcode.com/problems/merge-k-sorted-lists/
//
// Задача: даны k отсортированных по возрастанию связных списков. Слить их в один
// отсортированный список и вернуть его голову.
//
//   [1->4->5, 1->3->4, 2->6]  ->  1->1->2->3->4->4->5->6
//
// Идея: min-heap по головам списков. В куче всегда не более k узлов — по
// текущему минимуму из каждого списка. На каждом шаге достаём минимум, цепляем
// его в результат и кладём в кучу следующий узел из того же списка.
//
// Сложность: O(N log k) по времени (N — суммарное число узлов), O(k) по памяти
// на кучу.

// ListNode — узел односвязного списка.
type ListNode struct {
	Val  int
	Next *ListNode
}

// nodeHeap — min-heap указателей на узлы, упорядоченный по Val.
type nodeHeap []*ListNode

func (h nodeHeap) Len() int           { return len(h) }
func (h nodeHeap) Less(i, j int) bool { return h[i].Val < h[j].Val }
func (h nodeHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *nodeHeap) Push(x any)        { *h = append(*h, x.(*ListNode)) }
func (h *nodeHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

func MergeKLists(lists []*ListNode) *ListNode {
	h := &nodeHeap{}
	for _, node := range lists {
		if node != nil {
			*h = append(*h, node)
		}
	}
	heap.Init(h)

	// dummy упрощает присоединение узлов: не нужно отдельно обрабатывать голову.
	dummy := &ListNode{}
	tail := dummy
	for h.Len() > 0 {
		node := heap.Pop(h).(*ListNode)
		tail.Next = node
		tail = node
		if node.Next != nil {
			heap.Push(h, node.Next)
		}
	}
	return dummy.Next
}
