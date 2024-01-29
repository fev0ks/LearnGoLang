package main

import (
	"fmt"
	"runtime"
)

func main() {
	//slicer()
	extendSlice()
	//interfaceSlice()
	//memLeak()

}

func interfaceSlice() {
	var s []interface{}
	fmt.Println(s, len(s), cap(s))
	fmt.Printf("%p\n", s)
	//*s = append(*s, "w")
	s = append(s, "qwe")
	add(&s)
	add(&s)
	fmt.Println(s, len(s), cap(s))
	fmt.Printf("%p\n", s)
	fmt.Println(s[0].(string))
	v, ok := s[0].(int)
	fmt.Println(v, ok)
	fmt.Println()

	s2 := make([]interface{}, 0)
	fmt.Println(s2, len(s2), cap(s2))
	fmt.Printf("%p\n", s2)
	add(&s2)
	add(&s2)
	fmt.Println(s2, len(s2), cap(s2))
	fmt.Printf("%p\n", s2)
	fmt.Println()

	var sl = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
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
}

func slicer() {
	ints := make([]int, 1, 2)
	//fmt.Println(ints) // че выведет
	fmt.Printf("1ints %v l %d, c %d\n", ints, len(ints), cap(ints))
	appendSlice(ints, 1024)
	fmt.Printf("2ints %v l %d, c %d\n", ints, len(ints), cap(ints))
	//fmt.Println(ints) // че тут
	intsExp := ints[:2]
	fmt.Printf("intsExp %v l %d, c %d\n", intsExp, len(intsExp), cap(intsExp))
	//1ints [0] l 1, c 2
	//intSlice [0 1024] l 2, c 2
	//2ints [0] l 1, c 2
	//intsExp [0 1024] l 2, c 2
	fmt.Println()
	kek := [10]int{}
	fmt.Printf("Kek %v l %d, c %d\n", kek, len(kek), cap(kek)) // 10 10
	kek2 := kek[:]
	fmt.Printf("Kek2 %v l %d, c %d\n", kek2, len(kek2), cap(kek2)) // 10 10
	kek3 := kek[4:]
	fmt.Printf("kek3 %v l %d, c %d\n", kek3, len(kek3), cap(kek3)) // 6 6
	kek4 := kek[4:8]
	fmt.Printf("kek4 %v l %d, c %d\n", kek4, len(kek4), cap(kek4)) // 4 6
	fmt.Println(kek4[3])
}
func appendSlice(intSlice []int, val int) {
	intSlice = append(intSlice, val)
	fmt.Printf("intSlice %v l %d, c %d\n", intSlice, len(intSlice), cap(intSlice))
}

func extendSlice() {
	init := [5]string{"A", "B", "C", "D"}
	s1 := init[2:5]

	fmt.Printf("s1 %v, l%d, c%d\n", s1, len(s1), cap(s1))
	// output: (0xA1)["C","D",""], l=3, c=3

	updateSlice(s1)
	fmt.Printf("s1 updateSlice        %v, l%d, c%d\n", s1, len(s1), cap(s1))
	// pass by value   // output: (0xA1)["G","D",""], l=3, c=3

	updateSlicePointer(&s1)
	fmt.Printf("s1 updateSlicePointer %v, l%d, c%d\n", s1, len(s1), cap(s1))
	// pass by pointer // output: (0xD5)["G","D","H", "E"], l=4, c=6

	//s1 = append(s1, "E")
	//fmt.Println(s1, len(s1), cap(s1))
	// output: (0xD5)["C","D","", "E"], l=4, c=6
}

func updateSlicePointer(s1 *[]string) {
	s2 := *s1 // copy ref value

	fmt.Printf("s2[0] %p , s1[0] %p\n", &s2[0], &(*s1)[0]) // look at the same mem
	s2[0] = "G"
	s2 = append(s2, "E") // created new slice due to cap limit
	s2[2] = "H"
	*s1 = s2 // set ref s1 to s2 mem
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
	i := copy(s2, s[999_996:]) // allow GC to clear s slice
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
	// 1 2 3
	// 0 2 7 4
	// 0 2 7 4
	// 0 2 7 4 5
}
