package main

import "fmt"

func main() {
	//fmt.Println(test1())
	//test2()
}

func test1() (result int) {
	defer func() {
		result++
	}()
	return 123
}

func test2() {
	var i1 int = 10
	var k = 20
	var i2 *int = &k
	fmt.Printf("old k: %d %p\n", k, &k)
	fmt.Printf("old i2: %d %p\n", *i2, i2)

	defer printInt("i1", i1)
	defer printInt("i2 as a value", *i2)
	defer printPointer("i2 as a pointer", i2)

	i1 = 1010
	*i2 = 2020

	fmt.Printf("new k: %d %p\n", k, &k)
	fmt.Printf("new i2: %d %p\n", *i2, i2)
}

func printPointer(s string, i2 *int) {
	fmt.Printf("%s: %d %p\n", s, *i2, i2)
}

func printInt(s string, i1 int) {
	fmt.Printf("%s: %d %p\n", s, i1, &i1)
}
