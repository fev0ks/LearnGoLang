package merge_two_lists

// LeetCode 21. Merge Two Sorted Lists (Easy)
// https://leetcode.com/problems/merge-two-sorted-lists/
//
// Задача: даны два отсортированных по возрастанию связных списка. Слить их в
// один отсортированный список, переиспользуя существующие узлы.
//
//   1->2->4, 1->3->4  ->  1->1->2->3->4->4
//
// Идея: фиктивная голова (dummy) и хвост tail; на каждом шаге присоединяем
// меньшую из двух голов и сдвигаем её список. Хвост, оставшийся непустым,
// цепляем целиком.
//
// Сложность: O(n+m) по времени, O(1) по памяти.

// ListNode — узел односвязного списка.
type ListNode struct {
	Val  int
	Next *ListNode
}

func MergeTwoLists(l1, l2 *ListNode) *ListNode {
	dummy := &ListNode{}
	tail := dummy
	for l1 != nil && l2 != nil {
		if l1.Val <= l2.Val {
			tail.Next = l1
			l1 = l1.Next
		} else {
			tail.Next = l2
			l2 = l2.Next
		}
		tail = tail.Next
	}
	// Один из списков закончился — присоединяем остаток другого.
	if l1 != nil {
		tail.Next = l1
	} else {
		tail.Next = l2
	}
	return dummy.Next
}
