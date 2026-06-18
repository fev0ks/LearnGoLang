package main

import "testing"

func TestCountStrokes(t *testing.T) {
	cases := []struct {
		name string
		grid [][]int
		want int
	}{
		{
			// 5 связных одноцветных областей.
			name: "пример из условия",
			grid: [][]int{
				{1, 1, 1, 2, 1},
				{1, 2, 1, 2, 1},
				{1, 1, 1, 3, 1},
			},
			want: 5,
		},
		{
			name: "однотонная сетка — одна заливка",
			grid: [][]int{
				{7, 7},
				{7, 7},
			},
			want: 1,
		},
		{
			name: "все клетки разные",
			grid: [][]int{
				{1, 2},
				{3, 4},
			},
			want: 4,
		},
		{
			name: "одна клетка",
			grid: [][]int{{9}},
			want: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := countStrokes(c.grid); got != c.want {
				t.Errorf("countStrokes() = %d, want %d", got, c.want)
			}
		})
	}
}
