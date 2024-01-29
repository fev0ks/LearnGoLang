package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"reflect"
)

type A interface {
	say()
	say2()
}

type B struct {
}

func (b B) say() {
	fmt.Println("wooo")
}

func (b *B) say2() {
	fmt.Println("wooo2")
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

func main() {
	//nilInterface()
	//intUnderInt()
	testNilStructAsIntf()
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
	fmt.Printf("is a nil? %t %v\n\n", a == nil, reflect.TypeOf(a))

	b := &B{}
	a = b
	fmt.Printf("is a of b nil? %t %v\n", a == nil, reflect.TypeOf(a))
	fmt.Printf("is b nil? %t\n\n", b == nil)

	var c *C
	a = c
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
	a, ok := a2.(A) // A2 doesn't hide say2() of B type even A2 doesn't have this method
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
	write1(buf2) // p is Type '*bytes.Buffer', has value ''
	if buf2 != nil {
		fmt.Println(buf2)
	}

	var buf *bytes.Buffer // ? NewBuffer()
	write1(buf)           // p is Type *bytes.Buffer, has value '<nil>'
	if buf != nil {
		fmt.Println(buf)
	}
}

func write1(p io.Writer) { // io.Writer? - interface
	fmt.Printf("p is Type '%s', has value '%v'\n", reflect.TypeOf(p), reflect.ValueOf(p))
	if p != nil { // should actually prevent running
		_, err := p.Write([]byte("test"))
		if err != nil {
			log.Fatal(err.Error())
		}
	}
}
