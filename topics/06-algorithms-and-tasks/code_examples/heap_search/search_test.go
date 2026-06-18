package main

import (
	"reflect"
	"testing"
)

func TestTopKElements(t *testing.T) {
	cases := []struct {
		name string
		arr  []int
		k    int
		want []int // результат отсортирован по возрастанию (вершины min-heap)
	}{
		{"три из пяти", []int{5, 1, 9, 3, 7}, 3, []int{5, 7, 9}},
		{"k = длине", []int{2, 1, 3}, 3, []int{1, 2, 3}},
		{"k = 1", []int{4, 8, 2, 8, 1}, 1, []int{8}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TopKElements(c.arr, c.k); !reflect.DeepEqual(got, c.want) {
				t.Errorf("TopKElements(%v, %d) = %v, want %v", c.arr, c.k, got, c.want)
			}
		})
	}
}
