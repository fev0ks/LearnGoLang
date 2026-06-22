package smaller_numbers

import "sort"

// LeetCode 1365. How Many Numbers Are Smaller Than the Current Number (Easy)
// https://leetcode.com/problems/how-many-numbers-are-smaller-than-the-current-number/
//
// Задача: для каждого nums[i] посчитать, сколько других элементов массива строго
// меньше его. Вернуть массив тех же ответов в исходном порядке.
//
//	nums = [8, 1, 2, 2, 3] -> [4, 0, 1, 1, 3]
//	nums = [6, 5, 4, 8]    -> [2, 1, 0, 3]
//	nums = [7, 7, 7, 7]    -> [0, 0, 0, 0]

// SmallerNumbersThanCurrent для каждого nums[i] возвращает количество элементов
// массива, строго меньших его.
//
// Идея: по условию 0 <= nums[i] <= 100, поэтому используем counting sort вместо
// сортировки/двойного цикла. Считаем, сколько раз встречается каждое значение,
// затем префиксная сумма count[v] даёт количество элементов, строго меньших v.
// Для ответа берём count[nums[i]-1] (число значений < nums[i]).
//
// Сложность: O(n + K) по времени и O(K) по памяти, где K = 101 — диапазон
// значений. Это лучше наивного O(n^2) двойного цикла.
func SmallerNumbersThanCurrent(nums []int) []int {
	const maxVal = 100
	count := make([]int, maxVal+1) // count[v] — сколько раз встречается значение v
	for _, v := range nums {
		count[v]++
	}
	// Префиксная сумма: после прохода count[v] = число элементов со значением <= v.
	for v := 1; v <= maxVal; v++ {
		count[v] += count[v-1]
	}

	res := make([]int, len(nums))
	for i, v := range nums {
		if v > 0 {
			res[i] = count[v-1] // сколько элементов строго меньше v
		}
		// v == 0: меньше нечего, оставляем 0
	}
	return res
}

// SmallerNumbersGeneric решает ту же задачу, но не зависит от диапазона значений:
// работает с любыми int, включая отрицательные и очень большие. Нужна, когда
// ограничение 0 <= nums[i] <= 100 снято и counting sort применять нельзя (массив
// count размером с диапазон стал бы неподъёмным по памяти или невозможным для
// отрицательных индексов).
//
// Идея: отсортировать копию массива. Индекс первого вхождения значения в
// отсортированном массиве равен количеству элементов строго меньше него — всё,
// что левее, по определению меньше. Чтобы получать этот индекс за O(1), один раз
// проходим отсортированный массив справа налево и пишем в map значение -> индекс
// (левое перетирает правое, поэтому для дублей остаётся самый левый индекс).
//
// Сложность: O(n log n) по времени (сортировка), O(n) по памяти.
func SmallerNumbersGeneric(nums []int) []int {
	sorted := make([]int, len(nums))
	copy(sorted, nums)
	sort.Ints(sorted)

	firstIdx := make(map[int]int, len(sorted)) // значение -> индекс первого вхождения
	for i := len(sorted) - 1; i >= 0; i-- {
		firstIdx[sorted[i]] = i
	}

	res := make([]int, len(nums))
	for i, v := range nums {
		res[i] = firstIdx[v]
	}
	return res
}

// SmallerNumbersBinary — та же идея «индекс первого вхождения = число меньших»,
// но без map: для каждого элемента бинарным поиском ищем позицию, куда он встал
// бы в отсортированном массиве (sort.SearchInts возвращает первую такую позицию).
// Так же O(n log n) по времени, но без аллокации map — экономнее по памяти,
// ценой log n на каждый запрос вместо O(1).
func SmallerNumbersBinary(nums []int) []int {
	sorted := make([]int, len(nums))
	copy(sorted, nums)
	sort.Ints(sorted)

	res := make([]int, len(nums))
	for i, v := range nums {
		res[i] = sort.SearchInts(sorted, v) // первая позиция, куда встанет v
	}
	return res
}
