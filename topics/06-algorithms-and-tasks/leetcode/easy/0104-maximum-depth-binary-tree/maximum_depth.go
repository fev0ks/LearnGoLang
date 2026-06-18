package maximum_depth

// LeetCode 104. Maximum Depth of Binary Tree (Easy)
// https://leetcode.com/problems/maximum-depth-of-binary-tree/
//
// Задача: вернуть максимальную глубину бинарного дерева — число узлов на самом
// длинном пути от корня до листа.
//
//        3
//       / \
//      9   20
//          / \
//         15  7      -> 3
//
// Идея: глубина дерева = 1 + максимум из глубин левого и правого поддеревьев.
// Пустое дерево имеет глубину 0 (база рекурсии).
//
// Сложность: O(n) по времени, O(h) по памяти на стек рекурсии.

// TreeNode — узел бинарного дерева.
type TreeNode struct {
	Val   int
	Left  *TreeNode
	Right *TreeNode
}

func MaxDepth(root *TreeNode) int {
	if root == nil {
		return 0
	}
	left := MaxDepth(root.Left)
	right := MaxDepth(root.Right)
	if left > right {
		return left + 1
	}
	return right + 1
}
