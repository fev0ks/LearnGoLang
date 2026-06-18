package main

import "testing"

func TestFindReplaceIndex(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"((()", 1},     // "()()"
		{"(((((", -1},   // нечётная длина
		{"())", -1},     // нечётная длина
		{"(((()))(", 7}, // "(((())))"
		{")()(", -1},    // счётчики равны — одна замена их разбалансирует
		{")(", -1},      // замена даёт "((" или "))" — обе невалидны
		{"((", 1},       // "()"
		{"()", -1},      // уже правильная, но требуется ровно одна замена
		{"", -1},        // пустая
	}

	for _, c := range cases {
		t.Run(c.s, func(t *testing.T) {
			if got := findReplaceIndex(c.s); got != c.want {
				t.Errorf("findReplaceIndex(%q) = %d, want %d", c.s, got, c.want)
			}
		})
	}
}
