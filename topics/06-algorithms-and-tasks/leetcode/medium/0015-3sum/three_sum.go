package three_sum

import "sort"

// LeetCode 15. 3Sum (Medium)
// https://leetcode.com/problems/3sum/
//
// Задача: найти все уникальные тройки чисел, дающих в сумме 0.
//
//	[-1,0,1,2,-1,-4] -> [[-1,-1,2],[-1,0,1]]
//
// Идея: сортируем массив. Фиксируем первый элемент nums[i] и двумя указателями
// (left, right) ищем пару с суммой -nums[i]. Сортировка позволяет двигать
// указатели по знаку суммы и пропускать дубликаты, чтобы тройки не повторялись.
//
// Сложность: O(n^2) по времени, O(1) дополнительной памяти (без учёта ответа).
func ThreeSum(nums []int) [][]int {
	sort.Ints(nums)
	var res [][]int

	for i := 0; i < len(nums)-2; i++ {
		// Пропускаем повтор первого числа тройки.
		if i > 0 && nums[i] == nums[i-1] {
			continue
		}
		left, right := i+1, len(nums)-1
		for left < right {
			sum := nums[i] + nums[left] + nums[right]
			switch {
			case sum < 0:
				left++
			case sum > 0:
				right--
			default:
				res = append(res, []int{nums[i], nums[left], nums[right]})
				left++
				right--
				// Пропускаем дубликаты второго и третьего чисел.
				for left < right && nums[left] == nums[left-1] {
					left++
				}
				for left < right && nums[right] == nums[right+1] {
					right--
				}
			}
		}
	}
	return res
}
