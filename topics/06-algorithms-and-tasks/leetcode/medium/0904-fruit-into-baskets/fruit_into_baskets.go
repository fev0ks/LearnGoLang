package fruit_into_baskets

// LeetCode 904. Fruit Into Baskets (Medium)
// https://leetcode.com/problems/fruit-into-baskets/
//
// Задача: дан ряд деревьев, fruits[i] — тип фрукта на дереве i. Есть две
// корзины, в каждой — фрукты только одного типа. Двигаясь слева направо, нужно
// собрать максимум фруктов с непрерывного участка ряда. По сути: найти самый
// длинный непрерывный подмассив, содержащий не более ДВУХ различных значений, и
// вернуть его ДЛИНУ (количество фруктов).
//
//	[1, 2, 1]        -> 3   (весь ряд: типы {1,2})
//	[0, 1, 2, 2]     -> 3   ([1, 2, 2])
//	[1, 2, 3, 2, 2]  -> 4   ([2, 3, 2, 2])
//
// Идея — скользящее окно [left..right] с картой «тип -> сколько его в окне»:
//   - расширяем окно вправо;
//   - пока различных типов больше двух, сдвигаем left, уменьшая счётчики;
//   - на каждом шаге обновляем максимум длины окна.
//
// Сложность: O(n) по времени, O(1) по памяти (в карте не более трёх ключей).
func TotalFruits(fruits []int) int {
	counts := make(map[int]int)
	left, best := 0, 0

	for right := 0; right < len(fruits); right++ {
		counts[fruits[right]]++

		// Слишком много типов — ужимаем окно слева.
		for len(counts) > 2 {
			leftType := fruits[left]
			counts[leftType]--
			if counts[leftType] == 0 {
				delete(counts, leftType)
			}
			left++
		}

		if window := right - left + 1; window > best {
			best = window
		}
	}
	return best
}
