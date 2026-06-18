package course_schedule

import "testing"

func TestCanFinish(t *testing.T) {
	cases := []struct {
		name          string
		numCourses    int
		prerequisites [][]int
		want          bool
	}{
		{"одна зависимость", 2, [][]int{{1, 0}}, true},
		{"цикл из двух", 2, [][]int{{1, 0}, {0, 1}}, false},
		{"дерево зависимостей", 4, [][]int{{1, 0}, {2, 0}, {3, 1}, {3, 2}}, true},
		{"цикл из трёх", 3, [][]int{{0, 1}, {1, 2}, {2, 0}}, false},
		{"нет зависимостей", 3, nil, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CanFinish(c.numCourses, c.prerequisites); got != c.want {
				t.Errorf("CanFinish(%d, %v) = %v, want %v", c.numCourses, c.prerequisites, got, c.want)
			}
		})
	}
}
