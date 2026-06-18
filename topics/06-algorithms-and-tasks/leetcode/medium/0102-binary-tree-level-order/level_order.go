package level_order

// LeetCode 102. Binary Tree Level Order Traversal (Medium)
// https://leetcode.com/problems/binary-tree-level-order-traversal/
//
// Задача: обойти бинарное дерево по уровням (сверху вниз, слева направо) и
// вернуть значения, сгруппированные по уровням.
//
//        3
//       / \
//      9   20
//          / \
//         15  7      -> [[3],[9,20],[15,7]]
//
// Идея — обход в ширину (BFS) очередью. На каждой итерации фиксируем размер
// текущего уровня, обрабатываем ровно столько узлов, а их детей складываем в
// очередь для следующего уровня.
//
// Сложность: O(n) по времени и памяти.

// TreeNode — узел бинарного дерева.
type TreeNode struct {
	Val   int
	Left  *TreeNode
	Right *TreeNode
}

func LevelOrder(root *TreeNode) [][]int {
	if root == nil {
		return nil
	}

	var res [][]int
	queue := []*TreeNode{root}
	for len(queue) > 0 {
		count := len(queue) // число узлов на текущем уровне
		level := make([]int, 0, count)
		for i := 0; i < count; i++ {
			node := queue[i]
			level = append(level, node.Val)
			if node.Left != nil {
				queue = append(queue, node.Left)
			}
			if node.Right != nil {
				queue = append(queue, node.Right)
			}
		}
		queue = queue[count:] // отбрасываем обработанный уровень
		res = append(res, level)
	}
	return res
}
