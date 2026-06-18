package main

import (
	"math"
	"testing"
)

func TestFindMaxAverage(t *testing.T) {
	cases := []struct {
		name string
		nums []int
		k    int
		want float64
	}{
		{"пример из условия", []int{1, 12, -5, -6, 50, 3}, 4, 12.75},
		{"одно окно = весь массив", []int{5, 5, 5}, 3, 5},
		{"максимум в последнем окне", []int{1, 1, 1, 5}, 1, 5},
		{"k=1", []int{0, 4, 0, 3, 2}, 1, 4},
	}

	const eps = 1e-9
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Обе реализации должны совпадать с эталоном.
			if got := findMaxAverage(c.nums, c.k); math.Abs(got-c.want) > eps {
				t.Errorf("findMaxAverage(%v, %d) = %v, want %v", c.nums, c.k, got, c.want)
			}
			if got := findMaxAverage2(c.nums, c.k); math.Abs(got-c.want) > eps {
				t.Errorf("findMaxAverage2(%v, %d) = %v, want %v", c.nums, c.k, got, c.want)
			}
		})
	}
}
