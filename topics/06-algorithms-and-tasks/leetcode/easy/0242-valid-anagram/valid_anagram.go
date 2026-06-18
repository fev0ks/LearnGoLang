package valid_anagram

// LeetCode 242. Valid Anagram (Easy)
// https://leetcode.com/problems/valid-anagram/
//
// Задача: даны строки s и t из строчных латинских букв. Вернуть true, если t —
// анаграмма s (тот же мультимножество символов, возможно в другом порядке).
//
//	"anagram", "nagaram" -> true
//	"rat", "car"         -> false
//
// Идея: анаграммы имеют одинаковые счётчики каждого символа. Один массив на 26
// букв: для s увеличиваем, для t уменьшаем; если в конце все нули — анаграмма.
//
// Сложность: O(n) по времени, O(1) по памяти (фиксированные 26 счётчиков).
func IsAnagram(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	var counts [26]int
	for i := 0; i < len(s); i++ {
		counts[s[i]-'a']++
		counts[t[i]-'a']--
	}
	for _, c := range counts {
		if c != 0 {
			return false
		}
	}
	return true
}
