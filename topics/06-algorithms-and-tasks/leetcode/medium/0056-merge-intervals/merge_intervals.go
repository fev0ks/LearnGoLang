package merge_intervals

import "sort"

// LeetCode 56. Merge Intervals (Medium)
// https://leetcode.com/problems/merge-intervals/
//
// Задача: дан набор интервалов [start, end]. Слить все пересекающиеся и вернуть
// список непересекающихся интервалов.
//
//	[[1,3],[2,6],[8,10],[15,18]] -> [[1,6],[8,10],[15,18]]
//	[[1,4],[4,5]]                -> [[1,5]]   (касание считается пересечением)
//
// Идея: сортируем интервалы по началу. Идём слева направо; если очередной
// интервал начинается не позже конца последнего в результате — расширяем этот
// последний. Иначе добавляем как новый.
//
// Сложность: O(n log n) из-за сортировки, O(n) по памяти на результат.
func Merge(intervals [][]int) [][]int {
	if len(intervals) <= 1 {
		return intervals
	}

	sort.Slice(intervals, func(i, j int) bool {
		return intervals[i][0] < intervals[j][0]
	})

	merged := [][]int{{intervals[0][0], intervals[0][1]}}
	for _, cur := range intervals[1:] {
		last := merged[len(merged)-1]
		if cur[0] <= last[1] { // пересекаются или касаются
			if cur[1] > last[1] {
				last[1] = cur[1] // расширяем правую границу
			}
		} else {
			merged = append(merged, []int{cur[0], cur[1]})
		}
	}
	return merged
}
