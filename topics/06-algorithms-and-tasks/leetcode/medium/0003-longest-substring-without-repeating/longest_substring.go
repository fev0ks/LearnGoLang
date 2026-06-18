package longest_substring

// LeetCode 3. Longest Substring Without Repeating Characters (Medium)
// https://leetcode.com/problems/longest-substring-without-repeating-characters/
//
// Задача: найти длину самой длинной подстроки без повторяющихся символов.
//
//	"abcabcbb" -> 3   ("abc")
//	"bbbbb"    -> 1   ("b")
//	"pwwkew"   -> 3   ("wke")
//
// Идея — скользящее окно [left..right] + карта «символ -> последний индекс».
// Встречая повтор внутри окна, перепрыгиваем left за прошлое вхождение символа,
// чтобы в окне снова не было дубликатов. На каждом шаге обновляем максимум.
//
// Сложность: O(n) по времени, O(min(n, алфавит)) по памяти.
func LengthOfLongestSubstring(s string) int {
	lastSeen := make(map[byte]int) // символ -> индекс последнего вхождения
	left, best := 0, 0

	for right := 0; right < len(s); right++ {
		c := s[right]
		// Повтор внутри текущего окна — сдвигаем левую границу.
		if idx, ok := lastSeen[c]; ok && idx >= left {
			left = idx + 1
		}
		lastSeen[c] = right
		if w := right - left + 1; w > best {
			best = w
		}
	}
	return best
}
