package search_rotated

// LeetCode 33. Search in Rotated Sorted Array (Medium)
// https://leetcode.com/problems/search-in-rotated-sorted-array/
//
// Задача: отсортированный по возрастанию массив без повторов повёрнут в
// неизвестной точке (например, [0,1,2,4,5,6,7] -> [4,5,6,7,0,1,2]). Найти индекс
// target за O(log n) или вернуть -1.
//
//	[4,5,6,7,0,1,2], target=0 -> 4
//	[4,5,6,7,0,1,2], target=3 -> -1
//
// Идея: модифицированный бинарный поиск. На каждом шаге одна из половин
// (относительно mid) гарантированно отсортирована. Определяем какая и проверяем,
// попадает ли target в её диапазон — так выбираем, куда идти дальше.
//
// Сложность: O(log n) по времени, O(1) по памяти.
func Search(nums []int, target int) int {
	lo, hi := 0, len(nums)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		if nums[mid] == target {
			return mid
		}

		if nums[lo] <= nums[mid] { // левая половина отсортирована
			if nums[lo] <= target && target < nums[mid] {
				hi = mid - 1
			} else {
				lo = mid + 1
			}
		} else { // правая половина отсортирована
			if nums[mid] < target && target <= nums[hi] {
				lo = mid + 1
			} else {
				hi = mid - 1
			}
		}
	}
	return -1
}
