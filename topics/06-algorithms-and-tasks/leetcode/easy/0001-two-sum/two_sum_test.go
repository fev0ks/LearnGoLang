package two_sum

import (
	"reflect"
	"testing"
)

func TestTwoSum(t *testing.T) {
	cases := []struct {
		nums   []int
		target int
		want   []int
	}{
		{[]int{2, 7, 11, 15}, 9, []int{0, 1}},
		{[]int{3, 2, 4}, 6, []int{1, 2}},
		{[]int{3, 3}, 6, []int{0, 1}},
		{[]int{1, 2, 3}, 100, nil}, // решения нет
	}

	for _, c := range cases {
		if got := TwoSum(c.nums, c.target); !reflect.DeepEqual(got, c.want) {
			t.Errorf("TwoSum(%v, %d) = %v, want %v", c.nums, c.target, got, c.want)
		}
	}
}
