package merge_two_lists

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

func TestMergeTwoLists(t *testing.T) {
	cases := []struct {
		a, b, want []int
	}{
		{[]int{1, 2, 4}, []int{1, 3, 4}, []int{1, 1, 2, 3, 4, 4}},
		{nil, nil, nil},
		{nil, []int{0}, []int{0}},
		{[]int{1, 5}, []int{2, 3, 4}, []int{1, 2, 3, 4, 5}},
	}

	for _, c := range cases {
		got := toSlice(MergeTwoLists(fromSlice(c.a), fromSlice(c.b)))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("MergeTwoLists(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
