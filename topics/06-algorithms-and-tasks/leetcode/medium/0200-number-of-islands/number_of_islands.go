package number_of_islands

// LeetCode 200. Number of Islands (Medium)
// https://leetcode.com/problems/number-of-islands/
//
// Задача: дана матрица из '1' (суша) и '0' (вода). Посчитать число островов.
// Остров — группа клеток суши, связанных по горизонтали/вертикали.
//
//	1 1 0 0 0
//	1 1 0 0 0
//	0 0 1 0 0   -> 3 острова
//	0 0 0 1 1
//
// Идея: проходим по всем клеткам; встретив непосещённую сушу, увеличиваем
// счётчик и «топим» весь остров обходом в глубину (DFS), помечая клетки как
// воду, чтобы не считать их повторно.
//
// Сложность: O(rows*cols) по времени, O(rows*cols) на стек рекурсии в худшем
// случае. Замечание: функция изменяет переданную матрицу.
func NumIslands(grid [][]byte) int {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return 0
	}
	rows, cols := len(grid), len(grid[0])

	var sink func(r, c int)
	sink = func(r, c int) {
		if r < 0 || r >= rows || c < 0 || c >= cols || grid[r][c] != '1' {
			return
		}
		grid[r][c] = '0' // топим клетку
		sink(r+1, c)
		sink(r-1, c)
		sink(r, c+1)
		sink(r, c-1)
	}

	count := 0
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if grid[r][c] == '1' {
				count++
				sink(r, c)
			}
		}
	}
	return count
}
