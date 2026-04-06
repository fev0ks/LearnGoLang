package main

import (
	"fmt"
	"reflect"
	"testing"
)

func TestByte(t *testing.T) {
	count := 0
	for i := range [256]struct{}{} {
		if n := byte(i); n == -n {
			//n == byte(-int8(n))
			// 128 в int8 не влазит, получаем -128, делаем отрицание и получаем опять 128 в uint8 (byte)
			count++
			fmt.Println(reflect.TypeOf(n), reflect.TypeOf(-n)) //uint8 uint8
			fmt.Println(i, n, -n)                              //128 128 128
		}
	}
	println(count)
}

func o(b bool) bool {
	print(b)
	return !b
}

func TestBool(t *testing.T) {
	var x, y = true, false

	_ = x || o(x) // x == true, те в o(x) не зайдем, тк || уже сработал
	_ = y && o(y) // y == false, те o(y) не зайдем, тк && уже сработал
}

func TestSlice(t *testing.T) {
	fSlice()       // 0
	fSlice(nil)    // 1
	fSlice(nil...) //0
	v := []int{1, 2, 3}
	fSlice(v) //1 тк слайс 1 передали, а не его элементы
	v2 := []interface{}{1, 2, 3}
	fSlice(v2...) // 3, тк передали элементы слайса
}

func fSlice(vs ...interface{}) {
	println(len(vs)) // 4
}

func TestInt(t *testing.T) {
	println(fInt(3))
}

func fInt(n int) (r int) {
	a, r := n-1, n+1 // 2 4
	if a+a == r {
		c, r := n, n*n // 3 9 - НО r затеняется := и это другое r!
		r = r - c      // 6
	}
	// r = 4
	return r
}

func TestF(t *testing.T) {
	Bar() // 2
	print(" | ")
	Foo() // 210
}

var funcInt = func(x int) {}

func Bar() {
	funcInt := func(x int) {
		if x >= 0 {
			print(x)
			funcInt(x - 1)
		}
	}
	funcInt(2)
}

func Foo() {
	funcInt = func(x int) {
		if x >= 0 {
			print(x)
			funcInt(x - 1)
		}
	}
	funcInt(2)
}

func TestPtr(t *testing.T) {
	v := fIntrf() // not nil interface
	if v == nil {
		if xTestPtr == nil {
			println("A")
		} else {
			println("B")
		}
	} else if xTestPtr == nil { // nil
		println("C") // C
	} else {
		println("D")
	}
	var x1 *int
	fmt.Println(x1) // nil
	xV := 5
	x1 = &xV
	fmt.Println(reflect.TypeOf(x1)) // *int
	x2 := new(*int)
	fmt.Println(x2) // 0x14000096048
	x2 = &x1
	fmt.Println(reflect.TypeOf(x2)) // **int
	x3 := *new(*int)
	fmt.Println(x3) // <nil>
	x3 = &xV
	fmt.Println(reflect.TypeOf(x3)) // *int
}

var xTestPtr = *new(*int)
var yTestPtr *int = nil

func fIntrf() interface{} {
	return yTestPtr
}

func changePointer(pp **int) {
	newVal := 100
	*pp = &newVal
}

func TestChangePointer(t *testing.T) {
	var x int = 5
	p := &x
	fmt.Println(*p) // 5

	changePointer(&p)
	fmt.Println(*p) // 100
}
