package main

import (
	"fmt"
	"strconv"
)

func main() {
	a := 12 // 1100
	b := 10 // 1010 00001010

	fmt.Printf("%d & %d = %d\n", a, b, a&b)   // 8	(1000)
	fmt.Printf("%d | %d = %d\n", a, b, a|b)   // 14	(1110)
	fmt.Printf("%d ^ %d = %d\n", a, b, a^b)   // 6	(0110) XOR (исключающее ИЛИ)
	fmt.Printf("%d &^ %d = %d\n", a, b, a&^b) // 4	(0100) AND NOT (и не) Обнуляет биты правого операнда в левом
	fmt.Printf("%d |^ %d = %d\n", a, b, a|^b) // -3	(11111101)  битовый OR NOT
	// ^b = 11110101

	x := 3               // 0011
	fmt.Println(x << 2)  // 12  (1100)     3 * 2^2
	fmt.Println(12 >> 2) // 3  (0011) 12 \ 2^2

	fmt.Println(^uint8(0b00001111)) // 240 = 0b11110000 Побитовое НЕ - инвертирует
	fmt.Println(^a)                 // -13 = 0b11110000 Побитовое НЕ - инвертирует

	//a    = 00001100 (12)
	//^a   = 11110011 (-13)

	u := uint64(250)
	ub := strconv.FormatUint(u, 2)
	fmt.Println(ub) // 11111010

	fmt.Printf("%08b \n", uint8(a))  // 00001100
	fmt.Printf("%08b \n", uint8(^b)) // 00001100
	fmt.Printf("%08b \n", uint8(b))  // 00001100
}
