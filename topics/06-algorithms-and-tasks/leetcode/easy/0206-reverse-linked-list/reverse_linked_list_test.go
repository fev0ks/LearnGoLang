package reverse_linked_list

import (
	"reflect"
	"testing"
)

// fromSlice строит список из среза, toSlice — обратно, для удобных сравнений.
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

func TestReverseList(t *testing.T) {
	cases := []struct {
		in   []int
		want []int
	}{
		{nil, nil},
		{[]int{1}, []int{1}},
		{[]int{1, 2, 3, 4, 5}, []int{5, 4, 3, 2, 1}},
		{[]int{7, 7}, []int{7, 7}},
	}

	for _, c := range cases {
		got := toSlice(ReverseList(fromSlice(c.in)))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ReverseList(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
