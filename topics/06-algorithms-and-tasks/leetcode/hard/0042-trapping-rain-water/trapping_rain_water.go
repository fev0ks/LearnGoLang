package trapping_rain_water

// LeetCode 42. Trapping Rain Water (Hard)
// https://leetcode.com/problems/trapping-rain-water/
//
// Задача: дан массив высот столбиков (ширина каждого 1). Посчитать, сколько
// единиц воды удержится между ними после дождя.
//
//	[0,1,0,2,1,0,1,3,2,1,2,1] -> 6
//
// Идея — два указателя. Над каждым столбиком уровень воды определяется меньшим
// из двух максимумов (слева и справа). Двигаем тот указатель, со стороны
// которого максимум меньше: для него «потолок» уже точно известен, и можно сразу
// прибавить удержанную воду.
//
// Сложность: O(n) по времени, O(1) по памяти.
func Trap(height []int) int {
	left, right := 0, len(height)-1
	leftMax, rightMax := 0, 0
	water := 0

	for left < right {
		if height[left] < height[right] {
			// Слева потолок ниже — вода над left ограничена leftMax.
			if height[left] >= leftMax {
				leftMax = height[left]
			} else {
				water += leftMax - height[left]
			}
			left++
		} else {
			if height[right] >= rightMax {
				rightMax = height[right]
			} else {
				water += rightMax - height[right]
			}
			right--
		}
	}
	return water
}
