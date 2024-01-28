package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println(test1())
	//test2()
	//testCopy()
}

func testCopy() {
	start := time.Now()
	k := 1
	defer func() {
		fmt.Printf("defer 1\n")
		fmt.Println(start.Sub(time.Now())) // 3sec
		fmt.Println(k)                     // 4
	}()
	k = 2
	defer fmt.Println(start.Sub(time.Now())) // nanos
	defer fmt.Println(k)                     // 2
	defer fmt.Printf("defers 2\n")
	k = 3
	defer func(t time.Time, k2 int) {
		fmt.Printf("defer 3\n")
		fmt.Println(start.Sub(t)) // nanos
		fmt.Println(k2)           // 3
	}(time.Now(), k)
	k = 4
	time.Sleep(time.Second * 3)
	fmt.Println("end")
}

func test1() (result int) {
	defer func(result int) {
		fmt.Println(result) // 0
		result++
		fmt.Println(result) // 1
	}(result) // copy when defer init, doesn't affect final result, there is  = 1
	defer func() {
		fmt.Println(result) // 123
		result++
		fmt.Println(result) // 124
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
