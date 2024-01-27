package main

import (
	"fmt"
	"unicode/utf8"
)

func main() {
	str := "string with symbol 🛥."
	fmt.Println(utf8.RuneCountInString(str))
	fmt.Println(utf8.DecodeRune([]byte(str[19:23])))
	fmt.Println(str[19:23])

	for i, s := range str {
		fmt.Printf(" %d: %s-%d; ", i, string(s), s)
	}
}
