package main

import (
	"fmt"
)

var directions = [][]int{
	{0, 1},  // вправо
	{1, 0},  // вниз
	{0, -1}, // влево
	{-1, 0}, // вверх
}

func countStrokes(grid [][]int) int {
	rows := len(grid)
	cols := len(grid[0])
	visited := make([][]bool, rows)
	for i := range visited {
		visited[i] = make([]bool, cols)
	}

	strokes := 0

	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			if !visited[i][j] {
				dfs(grid, visited, i, j, grid[i][j])
				strokes++
			}
		}
	}

	return strokes
}

func dfs(grid [][]int, visited [][]bool, i, j, color int) {
	if i < 0 || i >= len(grid) || j < 0 || j >= len(grid[0]) {
		return
	}
	if visited[i][j] || grid[i][j] != color {
		return
	}

	visited[i][j] = true

	for _, dir := range directions {
		ni, nj := i+dir[0], j+dir[1]
		dfs(grid, visited, ni, nj, color)
	}
}

func main() {
	grid := [][]int{
		{1, 1, 1, 2, 1},
		{1, 2, 1, 2, 1},
		{1, 1, 1, 3, 1},
	}

	fmt.Println("Количество заливок:", countStrokes(grid)) // Ожидается: 5
}
