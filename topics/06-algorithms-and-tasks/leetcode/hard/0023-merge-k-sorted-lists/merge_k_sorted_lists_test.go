package merge_k_sorted_lists

import (
	"reflect"
	"testing"
)

func fromSlice(vals []int) *ListNode {
	dummy := &ListNode{}
	tail := dummy
	for _, v := range vals {
		tail.Next = &ListNode{Val: v}
		tail = tail.Next
	}
	return dummy.Next
}

func toSlice(head *ListNode) []int {
	var out []int
	for head != nil {
		out = append(out, head.Val)
		head = head.Next
	}
	return out
}

func TestMergeKLists(t *testing.T) {
	cases := []struct {
		name string
		in   [][]int
		want []int
	}{
		{
			name: "три списка",
			in:   [][]int{{1, 4, 5}, {1, 3, 4}, {2, 6}},
			want: []int{1, 1, 2, 3, 4, 4, 5, 6},
		},
		{
			name: "пустой вход",
			in:   nil,
			want: nil,
		},
		{
			name: "списки с пустыми",
			in:   [][]int{{}, {1}, {}},
			want: []int{1},
		},
		{
			name: "один список",
			in:   [][]int{{2, 4, 6}},
			want: []int{2, 4, 6},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lists := make([]*ListNode, len(c.in))
			for i, vals := range c.in {
				lists[i] = fromSlice(vals)
			}
			got := toSlice(MergeKLists(lists))
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("MergeKLists(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
