package symmetric_tree

import "testing"

// n — короткий хелпер для построения дерева в тестах.
func n(val int, left, right *TreeNode) *TreeNode {
	return &TreeNode{Val: val, Left: left, Right: right}
}

func leaf(val int) *TreeNode {
	return &TreeNode{Val: val}
}

func TestIsSymmetric(t *testing.T) {
	cases := []struct {
		name string
		root *TreeNode
		want bool
	}{
		{
			name: "nil дерево",
			root: nil,
			want: true,
		},
		{
			name: "один узел",
			root: leaf(1),
			want: true,
		},
		{
			//      1
			//     / \
			//    2   2
			//   / \ / \
			//  3  4 4  3
			name: "симметричное",
			root: n(1,
				n(2, leaf(3), leaf(4)),
				n(2, leaf(4), leaf(3)),
			),
			want: true,
		},
		{
			//      1
			//     / \
			//    2   2
			//     \   \
			//      3   3
			name: "значения зеркальны, но структура нет",
			root: n(1,
				n(2, nil, leaf(3)),
				n(2, nil, leaf(3)),
			),
			want: false,
		},
		{
			//      1
			//     / \
			//    2   2
			//   / \ / \
			//  3  4 3  4   (значения не зеркальны)
			name: "несимметричные значения",
			root: n(1,
				n(2, leaf(3), leaf(4)),
				n(2, leaf(3), leaf(4)),
			),
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsSymmetric(c.root); got != c.want {
				t.Errorf("IsSymmetric() = %v, want %v", got, c.want)
			}
			// Итеративный вариант должен давать тот же результат.
			if got := IsSymmetricIter(c.root); got != c.want {
				t.Errorf("IsSymmetricIter() = %v, want %v", got, c.want)
			}
		})
	}
}
