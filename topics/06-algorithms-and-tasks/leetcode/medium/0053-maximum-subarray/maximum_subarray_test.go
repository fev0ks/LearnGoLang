package maximum_subarray

import "testing"

func TestMaxSubArray(t *testing.T) {
	cases := []struct {
		nums []int
		want int
	}{
		{[]int{-2, 1, -3, 4, -1, 2, 1, -5, 4}, 6},
		{[]int{1}, 1},
		{[]int{5, 4, -1, 7, 8}, 23},
		{[]int{-1, -2, -3}, -1}, // все отрицательные — берём наибольший элемент
		{[]int{-5}, -5},
		{nil, 0},
	}
	for _, c := range cases {
		if got := MaxSubArray(c.nums); got != c.want {
			t.Errorf("MaxSubArray(%v) = %d, want %d", c.nums, got, c.want)
		}
	}
}
