package minimum_window

// LeetCode 76. Minimum Window Substring (Hard)
// https://leetcode.com/problems/minimum-window-substring/
//
// Задача: даны строки s и t. Найти наименьшую подстроку s, содержащую все
// символы t (с учётом кратности). Если такой нет — вернуть "".
//
//	s = "ADOBECODEBANC", t = "ABC" -> "BANC"
//	s = "a", t = "a"               -> "a"
//	s = "a", t = "aa"              -> ""
//
// Идея — скользящее окно. Расширяем правую границу, пока окно не покроет все
// нужные символы; затем сжимаем левую, пока покрытие держится, обновляя минимум.
// need[c] хранит, сколько символа c ещё требуется (может уходить в минус для
// «лишних»); missing — сколько символов осталось покрыть.
//
// Сложность: O(|s| + |t|) по времени, O(уникальных символов t) по памяти.
func MinWindow(s, t string) string {
	if len(t) == 0 || len(s) < len(t) {
		return ""
	}

	need := make(map[byte]int)
	for i := 0; i < len(t); i++ {
		need[t[i]]++
	}
	missing := len(t)

	left := 0
	bestStart, bestEnd := 0, 0 // лучший результат — полуинтервал [bestStart, bestEnd)
	for right := 0; right < len(s); right++ {
		if need[s[right]] > 0 {
			missing--
		}
		need[s[right]]--

		// Окно покрывает t — пытаемся сжать слева.
		for missing == 0 {
			if bestEnd == 0 || right-left+1 < bestEnd-bestStart {
				bestStart, bestEnd = left, right+1
			}
			need[s[left]]++
			if need[s[left]] > 0 {
				missing++ // убрали нужный символ — покрытие нарушилось
			}
			left++
		}
	}
	return s[bestStart:bestEnd]
}
