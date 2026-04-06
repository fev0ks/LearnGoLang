package main

import (
	"fmt"
	"time"
)

var grid = [][]int{
	{1, 1, 3, 3, 1},
	{1, 2, 3, 1, 1},
	{1, 1, 2, 4, 1},
	{1, 1, 1, 1, 1},
}

var visited [][]bool
var n, m int

var directions = [][]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}

func floodFill(x, y int, color int) {
	stack := [][2]int{{x, y}}
	original := grid[x][y]

	for len(stack) > 0 {
		now := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		i, j := now[0], now[1]
		if visited[i][j] {
			continue
		}
		visited[i][j] = true
		grid[i][j] = color
		printGrid()
		time.Sleep(200 * time.Millisecond)
		for _, d := range directions {
			nx, ny := i+d[0], j+d[1]
			if nx >= 0 && ny >= 0 && nx < n && ny < m && !visited[nx][ny] && grid[nx][ny] == original {
				stack = append(stack, [2]int{nx, ny})
			}
		}
	}
}

func printGrid() {
	fmt.Print("\033[H\033[2J") // Очистить терминал
	for _, row := range grid {
		for _, val := range row {
			fmt.Printf("%2d ", val)
		}
		fmt.Println()
	}
	fmt.Println()
}

func countFills() int {
	count := 0
	visited = make([][]bool, n)
	for i := range visited {
		visited[i] = make([]bool, m)
	}

	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			if !visited[i][j] {
				count++
				floodFill(i, j, count+10) // новые цвета с 11 и далее
			}
		}
	}
	return count
}

func main() {
	n = len(grid)
	m = len(grid[0])
	fmt.Println("Start flood filling...")
	strokes := countFills()
	fmt.Printf("Total strokes needed: %d\n", strokes)
}
