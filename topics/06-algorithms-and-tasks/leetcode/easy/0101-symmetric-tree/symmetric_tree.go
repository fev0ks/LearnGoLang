package symmetric_tree

// LeetCode 101. Symmetric Tree (Easy)
// https://leetcode.com/problems/symmetric-tree/
//
// Задача: дано бинарное дерево. Проверить, является ли оно зеркально
// симметричным относительно своего центра.
//
// Зеркальность означает: левое поддерево корня является зеркальным
// отражением правого поддерева.
//
//        1                  1
//       / \                / \
//      2   2              2   2
//     / \ / \              \   \
//    3  4 4  3             3    3
//   симметрично        НЕ симметрично
//
// Ключевая идея: сравниваем не одно дерево само с собой, а ДВА узла (зеркальную
// пару). Два дерева зеркальны, когда:
//   1) значения корней равны;
//   2) левое поддерево первого зеркально правому поддереву второго;
//   3) правое поддерево первого зеркально левому поддереву второго.
// Обратите внимание на крест-накрест: left<->right. В этом вся суть зеркала.

// TreeNode — узел бинарного дерева.
type TreeNode struct {
	Val   int
	Left  *TreeNode
	Right *TreeNode
}

// IsSymmetric — точка входа. Дерево симметрично, если его левое и правое
// поддеревья зеркальны друг другу.
//
// Сложность: O(n) по времени (каждый узел посещается один раз),
// O(h) по памяти на стек рекурсии, где h — высота дерева.
func IsSymmetric(root *TreeNode) bool {
	if root == nil {
		return true
	}
	return isMirror(root.Left, root.Right)
}

// isMirror — рекурсивно проверяет, что a и b являются зеркальными парами.
func isMirror(a, b *TreeNode) bool {
	// Оба пусты — границы совпали, ветки зеркальны.
	if a == nil && b == nil {
		return true
	}
	// Один пуст, другой нет — структура расходится.
	if a == nil || b == nil {
		return false
	}
	// Значения должны совпасть, а дети — сравниться крест-накрест.
	return a.Val == b.Val &&
		isMirror(a.Left, b.Right) &&
		isMirror(a.Right, b.Left)
}

// IsSymmetricIter — итеративный вариант без рекурсии (на случай, если
// на собеседовании просят избежать переполнения стека на глубоких деревьях).
// Кладём узлы в очередь зеркальными парами и обрабатываем их по два.
//
// Сложность: O(n) по времени, O(n) по памяти на очередь в худшем случае.
func IsSymmetricIter(root *TreeNode) bool {
	if root == nil {
		return true
	}

	queue := []*TreeNode{root.Left, root.Right}
	for len(queue) > 0 {
		// Достаём очередную зеркальную пару.
		a, b := queue[0], queue[1]
		queue = queue[2:]

		if a == nil && b == nil {
			continue
		}
		if a == nil || b == nil || a.Val != b.Val {
			return false
		}

		// Ставим в очередь внешние и внутренние пары — снова крест-накрест.
		queue = append(queue, a.Left, b.Right)
		queue = append(queue, a.Right, b.Left)
	}
	return true
}
