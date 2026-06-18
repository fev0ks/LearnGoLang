package search_rotated

import "testing"

func TestSearch(t *testing.T) {
	cases := []struct {
		nums   []int
		target int
		want   int
	}{
		{[]int{4, 5, 6, 7, 0, 1, 2}, 0, 4},
		{[]int{4, 5, 6, 7, 0, 1, 2}, 3, -1},
		{[]int{1}, 0, -1},
		{[]int{1}, 1, 0},
		{[]int{5, 1, 3}, 5, 0},
		{[]int{4, 5, 6, 7, 0, 1, 2}, 6, 2},
		{nil, 1, -1},
	}
	for _, c := range cases {
		if got := Search(c.nums, c.target); got != c.want {
			t.Errorf("Search(%v, %d) = %d, want %d", c.nums, c.target, got, c.want)
		}
	}
}
