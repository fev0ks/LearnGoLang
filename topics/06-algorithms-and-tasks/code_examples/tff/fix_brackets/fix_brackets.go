package main

import "fmt"

//Дана неправильная скобочная последовательность из
//открывающих и закрывающих скобок.
//Нужно выяснить, можно ли заменить одну из открывающих
//скобок на закрывающую или наоборот так, чтобы
//получилась правильная скобочная последовательность.
//Если можно, то вывести индекс скобки, которую надо
//заменить. Если нельзя, то вывести -1.

// ((() -> 1 ()() или 2 (())
// (((( -> -1
// ()) -> -1
// (((()))( -> 7 (((())))
// )()( -> -1

func main() {
	tests := []string{
		"((()",     // → 1 \\ 2
		"(((((",    // → -1
		"())",      // → -1
		"(((()))(", // → 7
		")()(",     // → -1
		")(",       // → -1
		"((",       // → 1
	}

	for _, test := range tests {
		fmt.Printf("Input: %-10s → Replace index: %d\n", test, findReplaceIndex(test))
	}
}

func findReplaceIndex(s string) int {
	n := len(s)
	if n%2 != 0 {
		return -1
	}

	balance := 0
	badIndex := -1
	problemCount := 0

	for i, ch := range s {
		if ch == '(' {
			balance++
		} else {
			balance--
		}

		if balance < 0 {
			// единственное место, где закрыли рано — возможно его можно заменить
			problemCount++
			if problemCount == 1 {
				badIndex = i
			}
		}
	}

	if balance == 0 && problemCount == 1 {
		// одна ошибка — замени её
		return badIndex
	}

	if balance == 2 && problemCount == 0 {
		// возможно, одна лишняя (
		openCount := 0
		for i, ch := range s {
			if ch == '(' {
				openCount++
			}
			if openCount > n/2 {
				return i
			}
		}
	}

	return -1
}
