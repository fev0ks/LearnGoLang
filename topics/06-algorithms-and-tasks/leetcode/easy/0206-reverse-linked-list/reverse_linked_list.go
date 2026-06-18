package reverse_linked_list

// LeetCode 206. Reverse Linked List (Easy)
// https://leetcode.com/problems/reverse-linked-list/
//
// Задача: развернуть односвязный список и вернуть новую голову.
//
//   1 -> 2 -> 3 -> 4 -> 5 -> nil   преобразуется в
//   5 -> 4 -> 3 -> 2 -> 1 -> nil
//
// Идея (итеративно): идём по списку, на каждом шаге разворачиваем ссылку
// текущего узла на предыдущий. Нужны три указателя: prev, текущий head и
// сохранённый next (чтобы не потерять хвост).
//
// Сложность: O(n) по времени, O(1) по памяти.

// ListNode — узел односвязного списка.
type ListNode struct {
	Val  int
	Next *ListNode
}

func ReverseList(head *ListNode) *ListNode {
	var prev *ListNode
	for head != nil {
		next := head.Next // запоминаем продолжение
		head.Next = prev  // разворачиваем ссылку
		prev = head       // сдвигаем prev
		head = next       // сдвигаем head
	}
	return prev // новая голова — бывший хвост
}
