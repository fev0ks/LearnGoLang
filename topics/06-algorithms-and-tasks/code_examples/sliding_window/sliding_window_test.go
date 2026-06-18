package sliding_window

import "testing"

func TestMaxSum(t *testing.T) {
	cases := []struct {
		name string
		nums []int
		want int
	}{
		{"пустой", nil, 0},
		{"один элемент", []int{5}, 5},
		{"два различных — всё окно", []int{1, 2}, 3},
		{"окно из двух значений в середине", []int{1, 10, 5, 10, 5, 10, 3, 10, 5}, 40},
		{"префикс из двух значений", []int{10, 10, 3, 5, 5, 5}, 23},
		{"все одинаковые", []int{4, 4, 4, 4}, 16},
		{"три различных подряд — окно сжимается", []int{1, 2, 3}, 5},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MaxSum(c.nums); got != c.want {
				t.Errorf("MaxSum(%v) = %d, want %d", c.nums, got, c.want)
			}
		})
	}
}
