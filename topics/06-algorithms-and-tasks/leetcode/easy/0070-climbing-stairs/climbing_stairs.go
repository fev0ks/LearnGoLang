package climbing_stairs

// LeetCode 70. Climbing Stairs (Easy)
// https://leetcode.com/problems/climbing-stairs/
//
// Задача: лестница из n ступеней, за раз можно подняться на 1 или 2 ступени.
// Сколькими разными способами можно подняться на вершину?
//
//	n = 2 -> 2   (1+1, 2)
//	n = 3 -> 3   (1+1+1, 1+2, 2+1)
//
// Идея: число способов дойти до ступени i = способы до (i-1) + способы до
// (i-2) — это числа Фибоначчи. Храним только два последних значения.
//
// Сложность: O(n) по времени, O(1) по памяти.
func ClimbStairs(n int) int {
	if n <= 2 {
		return n
	}
	prev, cur := 1, 2 // способы для n=1 и n=2
	for i := 3; i <= n; i++ {
		prev, cur = cur, prev+cur
	}
	return cur
}
