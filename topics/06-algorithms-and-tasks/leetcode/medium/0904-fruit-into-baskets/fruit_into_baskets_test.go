package fruit_into_baskets

import "testing"

func TestTotalFruits(t *testing.T) {
	cases := []struct {
		name   string
		fruits []int
		want   int
	}{
		{"пустой", nil, 0},
		{"один тип", []int{5, 5, 5}, 3},
		{"весь ряд из двух типов", []int{1, 2, 1}, 3},
		{"окно со второго элемента", []int{0, 1, 2, 2}, 3},
		{"окно в середине", []int{1, 2, 3, 2, 2}, 4},
		{"три типа по краям", []int{3, 3, 3, 1, 2, 1, 1, 2, 3, 3, 4}, 5},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TotalFruits(c.fruits); got != c.want {
				t.Errorf("TotalFruits(%v) = %d, want %d", c.fruits, got, c.want)
			}
		})
	}
}
