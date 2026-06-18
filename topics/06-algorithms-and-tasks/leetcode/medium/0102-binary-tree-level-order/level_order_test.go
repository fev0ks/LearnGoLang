package level_order

import (
	"reflect"
	"testing"
)

func TestLevelOrder(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := LevelOrder(nil); got != nil {
			t.Errorf("LevelOrder(nil) = %v, want nil", got)
		}
	})

	t.Run("три уровня", func(t *testing.T) {
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
		want := [][]int{{3}, {9, 20}, {15, 7}}
		if got := LevelOrder(root); !reflect.DeepEqual(got, want) {
			t.Errorf("LevelOrder() = %v, want %v", got, want)
		}
	})

	t.Run("один узел", func(t *testing.T) {
		want := [][]int{{1}}
		if got := LevelOrder(&TreeNode{Val: 1}); !reflect.DeepEqual(got, want) {
			t.Errorf("LevelOrder() = %v, want %v", got, want)
		}
	})
}
