package main

import (
	"fmt"
	"unsafe"
)

type s1 struct {
	b1 bool  //1 -> 8
	i  int64 //8
	b2 bool  //1 -> 8
} //24

type Example struct {
	A int32 // 4 байта
	B int8  // 1 байт
	C int8  // 1 байт
	D int16 // 2 байта
} //8

type Example2 struct {
	A int32 // 4 байта
	B int64 // 8 байт
	C int8  // 1 байт
	D int16 // 2 байта
} //24

type Example3 struct {
	A int32 // 4 байта
	B int64 // 8 байт
	C int8  // 1 байт
	D int16 // 2 байта
} //24

func main() {

	fmt.Printf("size Example: %d\n", unsafe.Sizeof(Example{}))
	fmt.Printf("size Example2: %d\n", unsafe.Sizeof(Example2{}))

	fmt.Printf("size 1: %d\n", unsafe.Sizeof(s1{}))

	var s2 struct {
		i  int64 // 8
		b1 bool  // 1
		b2 bool  // 1
	} // 16
	fmt.Printf("size 2: %d\n", unsafe.Sizeof(s2))

	var s3 struct {
		i  int32 //4
		b1 bool  //1
		b2 bool  //1
		s1 s1    //8 -> 24
	} // 32
	fmt.Printf("size 3: %d\n", unsafe.Sizeof(s3))
	fmt.Println(unsafe.Offsetof(s3.i))
	fmt.Println(unsafe.Offsetof(s3.b1))
	fmt.Println(unsafe.Offsetof(s3.b2))
	fmt.Println(unsafe.Offsetof(s3.s1))

	var s4 struct {
		i  int32 //4 -|
		b1 bool  //1 --> 8
		s1 s1    //8 -> 24
		b2 bool  //1 -> 8
	} // 40
	fmt.Printf("size 4: %d\n", unsafe.Sizeof(s4))
	fmt.Println(unsafe.Offsetof(s4.i))
	fmt.Println(unsafe.Offsetof(s4.b1))
	fmt.Println(unsafe.Offsetof(s4.s1))
	fmt.Println(unsafe.Offsetof(s4.b2))

	var s5 struct {
		i  int32        // 4 -> 8
		s  string       // 8ptr + 8len = 16
		ar [1000]string // 8+8 -> 1000 * 16
		b1 bool         // 1 -> 8
		s1 s1           // 8 -> 24
		b2 bool         // 1 -> 8
	}
	fmt.Printf("size 5: %d\n", unsafe.Sizeof(s5))
	fmt.Println("i", unsafe.Offsetof(s5.i))
	fmt.Println("s", unsafe.Offsetof(s5.s))
	fmt.Println("ar", unsafe.Offsetof(s5.ar))
	fmt.Println("b1", unsafe.Offsetof(s5.b1))
	fmt.Println("s1", unsafe.Offsetof(s5.s1))
	fmt.Println("b2", unsafe.Offsetof(s5.b2))

	// 4  (int32 i)
	//+ 4 (выравнивание)
	//+ 16 (string s)
	//+ 16000 ([1000]string)
	//+ 1 (bool b1)
	//+ 7 (выравнивание до 8)
	//+ 24 (s1)
	//+ 1 (bool b2)
	//+ 7 (выравнивание до 8)
	//--------------------
	//= 16064 байт

	var s6 struct {
		i  int32    // 4 -> 8
		s  string   // 8ptr + 8len = 16
		ar []string // 8ptr + 8len + 8cap = 24
		b1 bool     // 1 ->
		b2 bool     // 1 -> b1 + b2 = 8
	}
	fmt.Printf("size 6: %d\n", unsafe.Sizeof(s6))
	fmt.Println("i", unsafe.Offsetof(s6.i))
	fmt.Println("s", unsafe.Offsetof(s6.s))
	fmt.Println("ar", unsafe.Offsetof(s6.ar))
	fmt.Println("b1", unsafe.Offsetof(s6.b1))
	fmt.Println("b2", unsafe.Offsetof(s6.b2))

	//size 6: 56
	//i 0
	//s 8
	//ar 24
	//b1 48
	//b2 49
}
