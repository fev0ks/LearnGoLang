package maximum_subarray

// LeetCode 53. Maximum Subarray (Medium)
// https://leetcode.com/problems/maximum-subarray/
//
// Задача: найти непрерывный подмассив с максимальной суммой и вернуть эту сумму.
//
//	[-2,1,-3,4,-1,2,1,-5,4] -> 6   ([4,-1,2,1])
//
// Идея (алгоритм Кадане): идём слева направо, поддерживая сумму текущего
// подмассива cur. Если cur стал отрицательным, он только мешает — начинаем
// новый подмассив с текущего элемента. По пути запоминаем максимум.
//
// Сложность: O(n) по времени, O(1) по памяти.
func MaxSubArray(nums []int) int {
	if len(nums) == 0 {
		return 0
	}
	best, cur := nums[0], nums[0]
	for _, x := range nums[1:] {
		if cur < 0 {
			cur = x // прошлый подмассив тянул вниз — начинаем заново
		} else {
			cur += x
		}
		if cur > best {
			best = cur
		}
	}
	return best
}
