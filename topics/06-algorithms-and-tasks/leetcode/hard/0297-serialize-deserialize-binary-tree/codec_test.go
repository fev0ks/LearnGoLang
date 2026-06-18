package codec

import "testing"

func TestSerializeRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		root *TreeNode
	}{
		{"nil", nil},
		{"один узел", &TreeNode{Val: 1}},
		{
			//   1
			//  / \
			// 2   3
			//    / \
			//   4   5
			name: "с правым поддеревом",
			root: &TreeNode{
				Val:  1,
				Left: &TreeNode{Val: 2},
				Right: &TreeNode{
					Val:   3,
					Left:  &TreeNode{Val: 4},
					Right: &TreeNode{Val: 5},
				},
			},
		},
		{
			name: "отрицательные и перекос влево",
			root: &TreeNode{Val: -10, Left: &TreeNode{Val: -3, Left: &TreeNode{Val: -1}}},
		},
	}

	var c Codec
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s1 := c.Serialize(tc.root)
			// Round-trip: дерево -> строка -> дерево -> строка должно совпасть.
			s2 := c.Serialize(c.Deserialize(s1))
			if s1 != s2 {
				t.Errorf("round-trip разошёлся:\n  first = %q\n  again = %q", s1, s2)
			}
		})
	}
}

func TestSerializeFormat(t *testing.T) {
	var c Codec
	//   1
	//  / \
	// 2   3
	root := &TreeNode{Val: 1, Left: &TreeNode{Val: 2}, Right: &TreeNode{Val: 3}}
	want := "1,2,null,null,3,null,null"
	if got := c.Serialize(root); got != want {
		t.Errorf("Serialize() = %q, want %q", got, want)
	}
}
