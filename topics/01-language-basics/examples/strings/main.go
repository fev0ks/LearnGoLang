package main

import (
	"fmt"
	"unicode/utf8"
)

func main() {
	//k("0", "1", "2", "3")
	//str := "-kek"
	//fmt.Println(string(str[0]))
	//fmt.Println(str[1:])

	s := "123йцу世" // 1 1 1 2 2 2 3
	fmt.Printf("size of string: %d byte\n", len(s))
	fmt.Printf("len of string: %d\n", utf8.RuneCountInString(s))

	str := "Привет André!"

	// We can do this
	fmt.Println("string for as runes:")
	for _, v := range str {
		fmt.Printf("%c", v)
	}
	fmt.Printf("\n")

	fmt.Println(len(str)) //20
	fmt.Println(str[0:2]) // We can do this русские буквы = 2 байта
	fmt.Println(str[0:1]) // We can do this получим половину буквы П
	fmt.Println(str[0])   // 208

	dst := make([]byte, 4)
	copy(dst, str)                // We can do this
	fmt.Println(dst, string(dst)) // [208 159 209 128] Пр

	fmt.Printf("\n")
	//str = append(str, "a") // We cannot do this

	name := "Kek"
	age := 10
	fmt.Printf("name: %[1]s, age: %[2]d\n", name, age)
	fmt.Printf("возраст %[2]d, имя: %[1]s\n", name, age)

	strUpdate := "Привет"
	runes := []rune(strUpdate) // Преобразуем строку в массив рун
	runes[3] = 'М'             // Меняем 4-й символ ('в' → 'М')
	newStr := string(runes)
	fmt.Printf("\n")

	fmt.Printf("strUpdate: %s, %p\n", strUpdate, &strUpdate)
	fmt.Printf("newStr: %s, %p\n", newStr, &newStr) // "ПриМет"
}

func bigSymbol() {
	str := "string with symbol 🛥."
	fmt.Println(utf8.RuneCountInString(str))
	fmt.Println(utf8.DecodeRune([]byte(str[19:23])))
	fmt.Println(str[19:23])

	for i, s := range str {
		fmt.Printf(" %d: %s-%d; ", i, string(s), s)
	}
}

//func k(t string, tags ...string) {
//	p(tags...)
//}
//
//func p(tags []string) {
//	fmt.Println(tags)
//}
