package backspace_compare

// LeetCode 844. Backspace String Compare (Easy)
// https://leetcode.com/problems/backspace-string-compare/
//
// Задача: даны две строки s и t. Символ '#' означает backspace (удаление
// предыдущего набранного символа). Вернуть true, если после применения всех
// backspace'ов строки равны. Пустой буфер при '#' остаётся пустым.
//
//   s = "ab#c", t = "ad#c"  -> true  ("ac" == "ac")
//   s = "a##c", t = "#a#c"  -> true  ("c"  == "c")
//   s = "a#c",  t = "b"     -> false ("c"  != "b")
//
// Два подхода:
//   1) build — собрать итоговые строки через стек, сравнить. Просто, O(n) памяти.
//   2) twoPointers — идти с конца двумя указателями, пропуская удалённые
//      символы на лету. O(1) дополнительной памяти — этого обычно и ждут
//      на собеседовании как «оптимальный» вариант.

// BackspaceCompare — простой вариант через стек (build).
// Сложность: O(n+m) время, O(n+m) память.
func BackspaceCompare(s, t string) bool {
	return build(s) == build(t)
}

// build применяет backspace'ы и возвращает итоговую строку.
func build(s string) string {
	stack := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '#' {
			stack = append(stack, s[i])
		} else if len(stack) > 0 {
			stack = stack[:len(stack)-1] // backspace: снимаем верх стека
		}
	}
	return string(stack)
}

// BackspaceCompareTwoPointers — оптимальный вариант с O(1) доп. памяти.
// Идём с конца строк: встречая '#', увеличиваем «долг» пропусков; обычный
// символ либо «съедается» долгом, либо становится очередным значимым символом
// для сравнения.
func BackspaceCompareTwoPointers(s, t string) bool {
	i, j := len(s)-1, len(t)-1
	for i >= 0 || j >= 0 {
		i = nextValid(s, i)
		j = nextValid(t, j)

		// Обе строки исчерпаны одновременно — равны.
		if i < 0 && j < 0 {
			return true
		}
		// Одна кончилась раньше другой — не равны.
		if i < 0 || j < 0 {
			return false
		}
		if s[i] != t[j] {
			return false
		}
		i--
		j--
	}
	return true
}

// nextValid сдвигает индекс к ближайшему слева значимому символу, гася
// '#' соответствующим числом удалений.
func nextValid(s string, i int) int {
	skip := 0
	for i >= 0 {
		switch {
		case s[i] == '#':
			skip++
			i--
		case skip > 0:
			skip--
			i--
		default:
			return i
		}
	}
	return i
}
