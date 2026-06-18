package three_sum

import (
	"reflect"
	"testing"
)

func TestThreeSum(t *testing.T) {
	cases := []struct {
		name string
		nums []int
		want [][]int
	}{
		{
			"классический",
			[]int{-1, 0, 1, 2, -1, -4},
			[][]int{{-1, -1, 2}, {-1, 0, 1}},
		},
		{
			"нет троек",
			[]int{0, 1, 1},
			nil,
		},
		{
			"все нули",
			[]int{0, 0, 0, 0},
			[][]int{{0, 0, 0}},
		},
		{
			"короткий вход",
			[]int{1, -1},
			nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Результат детерминирован: сортировка входа задаёт порядок троек.
			if got := ThreeSum(c.nums); !reflect.DeepEqual(got, c.want) {
				t.Errorf("ThreeSum(%v) = %v, want %v", c.nums, got, c.want)
			}
		})
	}
}
