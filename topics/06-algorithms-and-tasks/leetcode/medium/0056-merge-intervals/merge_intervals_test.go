package merge_intervals

import (
	"reflect"
	"testing"
)

func TestMerge(t *testing.T) {
	cases := []struct {
		name string
		in   [][]int
		want [][]int
	}{
		{
			"классический",
			[][]int{{1, 3}, {2, 6}, {8, 10}, {15, 18}},
			[][]int{{1, 6}, {8, 10}, {15, 18}},
		},
		{
			"касание границ",
			[][]int{{1, 4}, {4, 5}},
			[][]int{{1, 5}},
		},
		{
			"неотсортированный вход",
			[][]int{{15, 18}, {2, 6}, {1, 3}, {8, 10}},
			[][]int{{1, 6}, {8, 10}, {15, 18}},
		},
		{
			"вложенный интервал",
			[][]int{{1, 10}, {2, 3}, {4, 5}},
			[][]int{{1, 10}},
		},
		{
			"один интервал",
			[][]int{{5, 7}},
			[][]int{{5, 7}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Merge(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Merge() = %v, want %v", got, c.want)
			}
		})
	}
}
