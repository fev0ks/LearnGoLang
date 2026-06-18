package maximum_depth

import "testing"

func TestMaxDepth(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := MaxDepth(nil); got != 0 {
			t.Errorf("MaxDepth(nil) = %d, want 0", got)
		}
	})

	t.Run("один узел", func(t *testing.T) {
		if got := MaxDepth(&TreeNode{Val: 1}); got != 1 {
			t.Errorf("MaxDepth(leaf) = %d, want 1", got)
		}
	})

	t.Run("несбалансированное", func(t *testing.T) {
		//   3
		//  / \
		// 9   20
		//     / \
		//    15  7
		root := &TreeNode{
			Val:  3,
			Left: &TreeNode{Val: 9},
			Right: &TreeNode{
				Val:   20,
				Left:  &TreeNode{Val: 15},
				Right: &TreeNode{Val: 7},
			},
		}
		if got := MaxDepth(root); got != 3 {
			t.Errorf("MaxDepth() = %d, want 3", got)
		}
	})

	t.Run("вырожденное в список", func(t *testing.T) {
		// 1 -> 2 -> 3 -> 4 (только левые потомки)
		root := &TreeNode{Val: 1, Left: &TreeNode{Val: 2, Left: &TreeNode{Val: 3, Left: &TreeNode{Val: 4}}}}
		if got := MaxDepth(root); got != 4 {
			t.Errorf("MaxDepth() = %d, want 4", got)
		}
	})
}
