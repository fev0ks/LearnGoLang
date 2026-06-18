package codec

import (
	"strconv"
	"strings"
)

// LeetCode 297. Serialize and Deserialize Binary Tree (Hard)
// https://leetcode.com/problems/serialize-and-deserialize-binary-tree/
//
// Задача: реализовать сериализацию бинарного дерева в строку и обратную
// десериализацию так, чтобы восстановленное дерево совпадало с исходным.
//
// Идея: префиксный обход (preorder) с явными маркерами "null" для пустых детей.
// Такой формат однозначно задаёт структуру: при десериализации читаем токены по
// порядку и рекурсивно строим узлы.
//
//        1
//       / \
//      2   3
//         / \
//        4   5    ->  "1,2,null,null,3,4,null,null,5,null,null"
//
// Сложность: O(n) по времени и памяти для обеих операций.

// TreeNode — узел бинарного дерева.
type TreeNode struct {
	Val   int
	Left  *TreeNode
	Right *TreeNode
}

// Codec не хранит состояния — методы оформлены на нём ради соответствия условию.
type Codec struct{}

// Serialize превращает дерево в строку из токенов, разделённых запятой.
func (c *Codec) Serialize(root *TreeNode) string {
	var sb strings.Builder
	var dfs func(*TreeNode)
	dfs = func(node *TreeNode) {
		if node == nil {
			sb.WriteString("null,")
			return
		}
		sb.WriteString(strconv.Itoa(node.Val))
		sb.WriteByte(',')
		dfs(node.Left)
		dfs(node.Right)
	}
	dfs(root)
	return strings.TrimSuffix(sb.String(), ",")
}

// Deserialize восстанавливает дерево из строки, полученной Serialize.
func (c *Codec) Deserialize(data string) *TreeNode {
	if data == "" {
		return nil
	}
	tokens := strings.Split(data, ",")
	idx := 0

	var build func() *TreeNode
	build = func() *TreeNode {
		token := tokens[idx]
		idx++
		if token == "null" {
			return nil
		}
		val, _ := strconv.Atoi(token)
		node := &TreeNode{Val: val}
		node.Left = build()
		node.Right = build()
		return node
	}
	return build()
}
