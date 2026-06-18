package number_of_islands

import "testing"

// toGrid превращает строки в матрицу байт — так тест-кейсы читаются нагляднее.
func toGrid(rows []string) [][]byte {
	grid := make([][]byte, len(rows))
	for i, r := range rows {
		grid[i] = []byte(r)
	}
	return grid
}

func TestNumIslands(t *testing.T) {
	cases := []struct {
		name string
		rows []string
		want int
	}{
		{
			name: "один большой остров",
			rows: []string{
				"11110",
				"11010",
				"11000",
				"00000",
			},
			want: 1,
		},
		{
			name: "три острова",
			rows: []string{
				"11000",
				"11000",
				"00100",
				"00011",
			},
			want: 3,
		},
		{
			name: "вся вода",
			rows: []string{"000", "000"},
			want: 0,
		},
		{
			name: "пусто",
			rows: nil,
			want: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NumIslands(toGrid(c.rows)); got != c.want {
				t.Errorf("NumIslands() = %d, want %d", got, c.want)
			}
		})
	}
}
