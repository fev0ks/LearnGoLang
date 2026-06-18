package simplify_path

import "strings"

// LeetCode 71. Simplify Path (Medium)
// https://leetcode.com/problems/simplify-path/
//
// Задача: дан абсолютный Unix-путь. Вернуть его каноническую форму:
//   - "."  — текущая директория, игнорируется;
//   - ".." — переход на уровень вверх (снимаем последний элемент, если он есть);
//   - несколько слэшей подряд трактуются как один;
//   - результат начинается с "/" и не заканчивается на "/" (кроме корня "/").
//
//   "/home/"            -> "/home"
//   "/../"              -> "/"
//   "/home//foo/"       -> "/home/foo"
//   "/a/./b/../../c/"   -> "/c"
//
// Идея: стек имён директорий. Разбиваем путь по "/", для ".." делаем Pop,
// "."/пустые пропускаем, остальное — Push. В конце склеиваем стек через "/".
//
// Сложность: O(n) по времени и памяти.

// Stack — обобщённый стек на односвязном списке (используется в SimplifyPath
// и пригодится как самостоятельный пример структуры данных).
type Stack[T any] struct {
	head *node[T]
}

type node[T any] struct {
	next  *node[T]
	value T
}

func (s *Stack[T]) Push(value T) {
	s.head = &node[T]{next: s.head, value: value}
}

func (s *Stack[T]) Pop() (value T, ok bool) {
	if s.head == nil {
		return value, false
	}
	value = s.head.value
	s.head = s.head.next
	return value, true
}

func (s *Stack[T]) Empty() bool {
	return s.head == nil
}

// SimplifyPath возвращает каноническую форму абсолютного Unix-пути.
func SimplifyPath(path string) string {
	stack := &Stack[string]{}
	for _, part := range strings.Split(path, "/") {
		switch part {
		case "", ".":
			// Пустой сегмент (//) или текущая директория — пропускаем.
			continue
		case "..":
			// Шаг вверх: снимаем верхний элемент, если он есть.
			stack.Pop()
		default:
			stack.Push(part)
		}
	}

	// Стек хранит элементы в обратном порядке (вершина — последняя директория),
	// поэтому разворачиваем при сборке результата.
	var parts []string
	for !stack.Empty() {
		v, _ := stack.Pop()
		parts = append([]string{v}, parts...)
	}
	return "/" + strings.Join(parts, "/")
}
