package main

import (
	"fmt"
	"time"
)

// Файл показывает две ключевые идеи:
// 1) defer выполняются в LIFO-порядке;
// 2) аргументы defer вычисляются сразу, а замыкание видит переменные позже.
func main() {
	//fmt.Println(test1())
	//test2()
	testCopy()
	//testTimeSpending()
}

func testCopy() {
	start := time.Now()
	k := 1
	defer func() {
		fmt.Printf("defer 1\n")
		fmt.Println(time.Now().Sub(start)) // около 3s: start захвачен по ссылке из внешней области
		fmt.Println(k)                     // 4: замыкание видит финальное значение k
	}()

	k = 2
	defer fmt.Println(start.Sub(time.Now())) // почти 0: time.Now() вычислился в момент объявления defer
	defer fmt.Println(k)                     // 2: аргумент тоже вычислился сразу
	defer fmt.Printf("defers 2\n")
	k = 3

	defer func(t time.Time, k2 int) {
		fmt.Printf("defer 3\n")
		fmt.Println(start.Sub(t)) // почти 0: t передан как значение в момент объявления defer
		fmt.Println(k2)           // 3: k на момент постановки defer был равен 3
	}(time.Now(), k)

	k = 4
	time.Sleep(time.Second * 3)
	fmt.Println("end")
}

func testTimeSpending() {
	defer printTimeSpend(time.Now()) // около 2s: timestamp вычислился сейчас, а вызов произошел позже
	defer func() {
		printTimeSpend(time.Now()) // почти 0: time.Now() вызовется только при выполнении defer
	}()
	time.Sleep(2 * time.Second)
}

func printTimeSpend(start time.Time) {
	fmt.Printf("time spent: %v\n", time.Since(start))
}

func test1() (result int) {
	defer func(result int) {
		fmt.Println(result) // 0
		result++
		fmt.Println(result) // 1
	}(result) // в параметр попала копия result на момент defer, на итоговый return она не влияет
	defer func() {
		fmt.Println(result) // 123
		result++
		fmt.Println(result) // 124: named return изменился прямо перед выходом из функции
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
	defer printInt("i2 as a value", *i2) // значение разыменовали сразу, это снимок числа 20
	defer printPointer("i2 as a pointer", i2) // сам указатель передали сразу, но читать значение будем позже

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
