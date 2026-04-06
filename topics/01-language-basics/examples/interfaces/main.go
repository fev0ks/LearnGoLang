package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"reflect"
)

// Файл про method set, typed nil внутри interface и различие value/pointer receiver.
func main() {
	//nilInterface()
	//intUnderInt()
	//testNilStructAsIntf()

	var a A
	b2 := B{}
	b2.say()
	b2.say2()
	b2.say3()

	a = &b2     // только *B реализует A, потому что say2 у B объявлен с pointer receiver
	a.say()     // b say
	a.say2()    // b ptr say2
	//a.say3()  // нельзя: метод say3 не входит в интерфейс A

	// Внутри interface лежит *B, а не B.
	// Прямая проверка `a.(B)` здесь паниковала бы.
	if bPtr, ok := a.(*B); ok {
		fmt.Printf("safe assert to *B: %T\n", bPtr) // *main.B
	}
}

type A interface {
	say()
	say2()
}

type B struct {
}

func (b B) say() {
	fmt.Println("b say")
}

func (b *B) say2() {
	fmt.Println("b ptr say2")
}

func (b *B) say3() {
	fmt.Println("b ptr say3")
}

type B2 struct {
	B
}

func (b B2) say() {
	fmt.Println("b2 say")
}

type B3 struct {
	B
	B2
}

type C struct {
	s  string
	s2 *string
}

func (c C) say() { // is not a * but works if &C{}
	c.s = "copy"
	fmt.Printf("say %s pointer %p\n", *c.s2, c.s2)
	//c.s2 = &c.s // new pointer
	*c.s2 = c.s // update value in pointer
	fmt.Printf("say %s pointer new %p\n", *c.s2, c.s2)
	fmt.Println("Oooo", c.s, *c.s2)
}

func (c *C) say2() {
	fmt.Println("Oooo2")
}

type D struct {
}

func (d D) say() {
	fmt.Println("Dddd")
}

func (d D) say2() {
	fmt.Println("Dddd2")
}

type A2 interface {
	say()
}

func implInterface() {
	var a A
	//a = B{} doesn't work
	a = &B{}
	a.say()
	a.say2()

	var a2 A
	c := &C{s: "qqq", s2: new(string)}
	a2 = c
	a2.say()
	a2.say2()
	fmt.Printf("%s pointer %p\n", *c.s2, c.s2)
	fmt.Println(c.s, *c.s2)
}

func nilInterface() {
	var a A
	fmt.Printf("is a nil? %t %v\n\n", a == nil, reflect.TypeOf(a)) // true <nil>

	b := &B{}
	a = b
	fmt.Printf("is a of b nil? %t %v\n", a == nil, reflect.TypeOf(a))
	fmt.Printf("is b nil? %t\n\n", b == nil)

	var c *C
	a = c
	// Здесь a != nil, потому что у interface уже есть dynamic type (*C),
	// хотя само pointer-значение внутри него nil.
	fmt.Printf("is a of c nil? %t %v\n", a == nil, reflect.TypeOf(a))
	fmt.Printf("is c nil? %t\n\n", c == nil)

	var d D
	a = d
	fmt.Printf("is a of d nil? %t %v\n", a == nil, reflect.TypeOf(a))
	//fmt.Printf("is d nil? %t\n", d == nil)  // d is not a pointer
}

func intUnderInt() {
	var a A
	var a2 A2

	b := &B{}
	a2 = b
	// Интерфейс A2 "скрывает" методы только на уровне статической типизации.
	// Динамический тип внутри a2 все равно *B, поэтому assert к A успешен.
	a, ok := a2.(A)
	if ok {
		a.say2()
	}
}

func testNilStructAsIntf() {
	//var myMap map[int]int // (&nil)
	// make()
	//myMap[0] = 1 // panic,

	// bug = &bytes.Buffer{}
	buf2 := new(bytes.Buffer)
	write1(buf2) // нормальный случай: в interface лежит ненулевой *bytes.Buffer
	if buf2 != nil {
		fmt.Println(buf2)
	}

	var buf *bytes.Buffer
	write1(buf) // хитрый случай: p != nil, потому что interface содержит type=*bytes.Buffer, value=nil
	if buf != nil {
		fmt.Println(buf)
	}
}

func write1(p io.Writer) { // io.Writer? - interface
	fmt.Printf("p is Type '%s', has value '%v'\n", reflect.TypeOf(p), reflect.ValueOf(p))
	if p != nil { // этого условия недостаточно, если внутри interface лежит typed nil
		_, err := p.Write([]byte("test"))
		if err != nil {
			log.Fatal(err.Error())
		}
	}
}
