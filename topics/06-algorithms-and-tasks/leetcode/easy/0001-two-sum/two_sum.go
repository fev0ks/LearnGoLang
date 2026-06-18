package two_sum

// LeetCode 1. Two Sum (Easy)
// https://leetcode.com/problems/two-sum/
//
// Задача: дан массив nums и число target. Вернуть индексы двух элементов, сумма
// которых равна target (ровно одно решение, один элемент дважды использовать
// нельзя).
//
//	nums = [2, 7, 11, 15], target = 9  -> [0, 1]   (2 + 7)
//	nums = [3, 2, 4],       target = 6  -> [1, 2]   (2 + 4)
//
// Идея: одним проходом. Для текущего x проверяем, встречали ли уже дополнение
// target-x; если да — пара найдена. Иначе запоминаем x с его индексом.
//
// Сложность: O(n) по времени, O(n) по памяти на map.
func TwoSum(nums []int, target int) []int {
	seen := make(map[int]int, len(nums)) // значение -> индекс
	for i, x := range nums {
		if j, ok := seen[target-x]; ok {
			return []int{j, i}
		}
		seen[x] = i
	}
	return nil // по условию не достигается, но возвращаем явно
}
