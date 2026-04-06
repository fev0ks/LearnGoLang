package main

import (
	"fmt"
	"reflect"
	"runtime"
	"slices"
)

// Файл собирает типичные ловушки со slice:
// shared backing array, append, copy, nil slice и memory retention.
func main() {
	Tupanul()
	//refSlice()
	//slicer()
	//extendSlice()
	//interfaceSlice()
	//memLeak()
	//subSLice()
	//_ = stackSlice()
	//_ = stackSlice2()

}

func refSlice() {
	type vector2 [2]float32
	v1 := vector2{1, 2} // array копируется по значению целиком
	v2 := v1
	v2[0] = 3
	fmt.Printf("v1 = %v\n", v1) // 1 2
	fmt.Printf("v2 = %v\n", v2) // 3 2

	type vectorN []float32 // slice сам по себе маленький заголовок над backing array
	v3 := vectorN{1, 2}
	v4 := v3
	v4[0] = 4
	fmt.Println(v3) // 4 2
	fmt.Println(v4) // 4 2
	fmt.Println()
	v4 = append(v4, 5) // append может перевыделить массив, и после этого s3/s4 уже не обязаны делить память
	v3[1] = 1
	fmt.Println(v3) // 4 1
	fmt.Println(v4) // 4 2 5
}

func interfaceSlice() {
	var s []interface{}
	fmt.Println(s, len(s), cap(s))
	fmt.Printf("%p\n", s)
	//*s = append(*s, "w")
	s = append(s, "qwe")
	add(&s)
	add(&s)
	fmt.Println(s, len(s), cap(s)) // [qwe 1 1] 3 4
	fmt.Printf("%p\n", s)
	fmt.Println(s[0].(string)) // qwe
	v, ok := s[0].(int)
	fmt.Println(v, ok) // 0 false
	fmt.Println()

	s2 := make([]interface{}, 0)
	fmt.Println(s2, len(s2), cap(s2))
	fmt.Printf("%p\n", s2)
	add(&s2)
	add(&s2)
	add(&s2)
	fmt.Println(s2, len(s2), cap(s2)) // [1 1 1] 3 4
	fmt.Printf("%p\n", s2)
	fmt.Println()

	var sl = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
	fmt.Println("sl type:", reflect.TypeOf(sl)) // []int (slice)
	//var sarr = [3]int(sl) // cannot convert s (variable of type []int) to type [3]int
	fmt.Printf("sl %v, len %d. cap %d \n", sl, len(sl), cap(sl)) // sl [1 2 3 4 5 6 7 8 9 0], len 10. cap 10
	var sarr1 = (*[7]int)(sl)
	var res = *(*[3]int)(sl)
	sarr1[0] = 999
	fmt.Printf("sarr1 %v, len %d. cap %d \n", sarr1, len(sarr1), cap(sarr1)) // sarr1 &[999 2 3 4 5 6 7], len 7. cap 7
	fmt.Printf("res %v, len %d. cap %d \n", res, len(res), cap(res))         // res [1 2 3], len 3. cap 3
	fmt.Printf("&sl[0] = %p\n", &sl[0])                                      // &sl[0] = 0xc0000181e0
	fmt.Printf("&sarr1[0] = %p\n", &sarr1[0])                                // &sarr1[0] = 0xc0000181e0
	fmt.Printf("&res[0] = %p\n", &res[0])                                    // &res[0] = 0xc000010168
	fmt.Printf("sl %v, len %d. cap %d \n", sl, len(sl), cap(sl))             // sl [999 2 3 4 5 6 7 8 9 0], len 10. cap 10
	fmt.Println()

	var sl2 = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
	var s3 = sl2[2:3]                                                   // len 1 cap 8 (10-2)
	fmt.Printf("s3 %v %p, len %d. cap %d \n", s3, s3, len(s3), cap(s3)) // s3 [3] 0xc000018290, len 1. cap 8
	s3 = append(s3, 4)
	s3 = append(s3, 4)
	s3 = append(s3, 4)
	fmt.Printf("s3 %v, len %d. cap %d \n", s3, len(s3), cap(s3))     // s3 [3 4 4 4], len 4. cap 8
	fmt.Printf("sl2 %v, len %d. cap %d \n", sl2, len(sl2), cap(sl2)) // sl2 [1 2 3 4 4 4 7 8 9 0], len 10. cap 10
	fmt.Printf("%p\n", s3)
	fmt.Println()

	m := make([]int, 10)
	fmt.Printf("m %v, len %d. cap %d \n", m, len(m), cap(m)) // m [0 0 0 0 0 0 0 0 0 0], len 10. cap 10

	s4 := append(sl, sl...)
	fmt.Println(s4, len(s4), cap(s4)) // [999 2 3 4 5 6 7 8 9 0 999 2 3 4 5 6 7 8 9 0] 20 20
}

func add(s *[]interface{}) {
	*s = append(*s, 1)
	//s = append(s, 1) //Cannot use 's' (type *[]interface{}) as the type []Type
}

func Tupanul() {
	s := []int{1, 2, 3, 4, 5}
	fmt.Println(s) // [1 2 3 4 5]
	s1 := append(s, 6) // здесь cap у s не хватает, поэтому создается новый backing array
	s1[0] = 11
	fmt.Println(s)  // [1 2 3 4 5]
	fmt.Println(s1) // [11 2 3 4 5 6]
	s2 := append(s1, 7) // здесь append уже может переиспользовать backing array s1
	s2[0] = 12
	fmt.Println(s1) // [12 2 3 4 5 6]
	fmt.Println(s2) // [12 2 3 4 5 6 7]
	var sCopy []int
	copy(sCopy, s2) // ничего не копируется: len(sCopy) == 0
	s2[1] = 12
	fmt.Println(s1)    // [12 12 3 4 5 6]
	fmt.Println(sCopy) // [] - sCopy без cap, а copy копирует по мин cap из пары

	sCopy2 := make([]int, len(s2))
	copy(sCopy2, s2)
	s2[2] = 12
	fmt.Println(s1)     // [12 12 12 4 5 6]
	fmt.Println(sCopy2) // [12 12 3 4 5 6 7]

	s3 := append(s2, 8)
	s3[6] = 17
	// s2 и s3 еще смотрят в один и тот же backing array.
	// Поэтому s2 "видит" изменение элемента с индексом 6, но не "знает" про новый len у s3.
	fmt.Println(s2, len(s2), cap(s2)) // [12 12 12 4 5 6 17] 7 10
	fmt.Println(s3, len(s3), cap(s3)) // [12 12 12 4 5 6 17 8] 8 10
}

func slicer() {
	nilSl := []string(nil)
	fmt.Printf("nilSl %v l %d, c %d\n", nilSl, len(nilSl), cap(nilSl)) // nilSl [] l 0, c 0
	fmt.Printf("is nilSl nil - %t\n", nilSl == nil)                    // true
	nilSl2 := append(nilSl, nilSl...)
	fmt.Printf("nilSl2 %v l %d, c %d\n", nilSl2, len(nilSl2), cap(nilSl2)) // nilSl2 [] l 0, c 0
	fmt.Printf("is nilSl2 nil - %t\n", nilSl2 == nil)                      // true
	nilSl3 := append(nilSl, []string{""}...)
	fmt.Printf("nilSl3 %v l %d, c %d\n", nilSl3, len(nilSl3), cap(nilSl3)) // nilSl3 [] l 1, c 1
	fmt.Printf("is nilSl3 nil - %t\n", nilSl3 == nil)                      // false

	ints := make([]int, 1, 2)
	// Внутри backing array уже есть место под второй элемент, но len пока равен 1.
	fmt.Printf("1ints %v l %d, c %d\n", ints, len(ints), cap(ints))
	appendSlice(ints, 1024)
	// В appendSlice изменился только локальный заголовок slice.
	// Но сам backing array общий, поэтому через reslice можно увидеть записанный элемент.
	fmt.Printf("2ints %v l %d, c %d\n", ints, len(ints), cap(ints)) // [0] l 1, c 2
	intsExp := ints[:2]
	fmt.Printf("intsExp %v l %d, c %d\n", intsExp, len(intsExp), cap(intsExp))
	//1ints [0] l 1, c 2
	//intSlice [0 1024] l 2, c 2
	//2ints [0] l 1, c 2
	//intsExp [0 1024] l 2, c 2
	fmt.Println()
	kek := [10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
	fmt.Printf("Kek %v l %d, c %d\n", kek, len(kek), cap(kek)) // 10 10
	kek2 := kek[:]
	fmt.Printf("Kek2 %v l %d, c %d\n", kek2, len(kek2), cap(kek2)) // 10 10
	kek3 := kek[4:]
	fmt.Printf("kek3 %v l %d, c %d\n", kek3, len(kek3), cap(kek3)) // 6 6
	kek4 := kek[4:8]
	fmt.Printf("kek4 %v l %d, c %d\n", kek4, len(kek4), cap(kek4)) //[5 6 7 8] 4 6
	fmt.Println(kek4[3])                                           // 8
	kek5 := kek[4:8:9]
	fmt.Printf("kek5 %v l %d, c %d\n", kek5, len(kek5), cap(kek5)) // kek5 [5 6 7 8] l 4, c 5
}

func appendSlice(intSlice []int, val int) {
	intSlice = append(intSlice, val)
	fmt.Printf("intSlice %v l %d, c %d\n", intSlice, len(intSlice), cap(intSlice))
}

func extendSlice() {
	init := [5]string{"A", "B", "C", "D"}
	s1 := init[2:5]

	fmt.Printf("s1 %v, l%d, c%d\n", s1, len(s1), cap(s1))
	// len=3, cap=3: в slice попали элементы ["C", "D", ""].

	updateSlice(s1)
	fmt.Printf("s1 updateSlice        %v, l%d, c%d\n", s1, len(s1), cap(s1))
	// s1[0] поменялся на "G", потому что updateSlice менял общий backing array.
	// Но append внутри updateSlice создал новый slice, и этот новый заголовок потерялся.

	updateSlicePointer(&s1)
	fmt.Printf("s1 updateSlicePointer %v, l%d, c%d\n", s1, len(s1), cap(s1))
	// Здесь передали указатель на slice header, поэтому функция смогла вернуть новый len/cap наружу.

	//s1 = append(s1, "E")
	//fmt.Println(s1, len(s1), cap(s1))
	// output: (0xD5)["C","D","", "E"], l=4, c=6
}

func updateSlicePointer(s1 *[]string) {
	s2 := *s1 // копируем только header slice, а не данные

	fmt.Printf("s2[0] %p , s1[0] %p\n", &s2[0], &(*s1)[0]) // адрес одинаковый: данные пока общие
	s2[0] = "G"
	s2 = append(s2, "E") // cap не хватает -> получаем новый backing array
	s2[2] = "H"
	*s1 = s2 // теперь внешний slice начинает смотреть на новый backing array
}

func updateSlice(s2 []string) {
	s2[0] = "G"
	s2 = append(s2, "E") // this new address is lost
	s2[2] = "H"
}

func memLeak() {
	s := getSubSlice()
	fmt.Println(len(s), cap(s))
	printMemStat()

	all := make([][]int, 0)
	all = append(all, s)

	for i := 1; i < 10; i++ {
		s2 := getSubSlice()
		runtime.GC()
		printMemStat()

		all = append(all, s2)
	}

	runtime.GC()
	printMemStat()

	fmt.Println(all) // [[6 7] [6 7] [6 7] [6 7] [6 7] [6 7] [6 7] [6 7] [6 7] [6 7]]
}

func getSubSlice() []int {
	s := make([]int, 1_000_000)
	s[999997] = 7
	s[999996] = 6
	//fmt.Println(len(s), cap(s))
	s2 := make([]int, 2)
	i := copy(s2, s[999_996:]) // копируем только хвост; большой исходный массив после этого можно освободить GC
	fmt.Printf("i: %d\n", i)
	fmt.Println(len(s2), cap(s2))
	return s2
}

func printMemStat() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Println(m.Alloc / 1024 / 1024)
}

func testSlices1() {
	a := []string{"a", "b", "c"}
	b := a[1:2]
	b[0] = "q"

	fmt.Printf("%s\n", a) // что отобразится после вызова?
}

func testSlices2() {
	a := []string{"a", "b", "c"}
	fmt.Printf("a %d %d\n", len(a), cap(a))
	c := a[1:1]
	fmt.Printf("c %d %d\n", len(c), cap(c))
	b := append(a[1:2], "d") // start point of b is reference to a's "b" mem
	//b = append(b, "c") // this row will copy our array b to a new space
	fmt.Printf("b %d %d\n", len(b), cap(b))
	fmt.Printf("b %v\n", b) // b [b d]
	b[0] = "z"
	fmt.Printf("b %v\n", b) // b [z d]

	fmt.Printf("%v\n", a) // [a z d]
}

func testSlices3() {
	a := []int{1, 2, 3} // l3 c3
	b := a
	b = append(b, 4)  // l4 c6
	c := b            // 4 6
	b[0] = 0          // 0 2 3 4
	e := append(c, 5) // 0 2 3 4 5; l5 c6
	b[2] = 7          // 0 2 7 4

	fmt.Println(a, b, c, e)
	slices.Reverse(a)
	// 1 2 3
	// 0 2 7 4
	// 0 2 7 4
	// 0 2 7 4 5
}

func subSLice() {
	sl1 := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	fmt.Printf("sl1: %v, l %d, c %d\n", sl1, len(sl1), cap(sl1))

	sl2 := make([]int, 3)
	sl2 = append(sl2, 123)
	fmt.Printf("sl2: %v, l %d, c %d\n", sl2, len(sl2), cap(sl2))

	sl21 := make([]int, 0, 3)
	sl21 = append(sl21, 123)
	fmt.Printf("sl21: %v, l %d, c %d\n", sl21, len(sl21), cap(sl21))

	slSub := make([]int, 3)
	fmt.Printf("slSub: %v, l %d, c %d\n\n", slSub, len(slSub), cap(slSub))
	slSub = sl1[3:5] // cap от начала и до конца sl1 = 10-3 = 7
	fmt.Printf("slSub copy 3:5 : %v, l %d, c %d\n\n", slSub, len(slSub), cap(slSub))

	slSub[0] = 100
	slSub = append(slSub, 123)
	fmt.Printf("slSub: %v, l %d, c %d\n", slSub, len(slSub), cap(slSub))
	fmt.Printf("updated sl1: %v, l %d, c %d\n", sl1, len(sl1), cap(sl1))

	sl1 = append(sl1, 5)
	slSub[1] = 101
	fmt.Printf("updated sl1: %v, l %d, c %d\n", sl1, len(sl1), cap(sl1))
}

func stackSlice() []int {
	var arr [1]int
	arr[0] = 1
	//arr[1] = 2
	//arr[2] = 3
	return arr[:]
}

func stackSlice2() int {
	var arr2 [1]int
	arr2[0] = 1
	//arr[1] = 2
	//arr[2] = 3
	return arr2[0]
}
