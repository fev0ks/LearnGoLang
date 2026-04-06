package main

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
)

// Главная идея примера:
// interface может быть "не nil", даже если внутри лежит nil-указатель конкретного типа.
func main() {
	//buf := &bytes.Buffer{} // обычный безопасный случай
	var buf *bytes.Buffer // nil-указатель конкретного типа *bytes.Buffer

	write1(buf)
	write2(buf)
	if buf != nil {
		fmt.Println(buf)
	}
}

func write1(p io.Writer) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("write1: panic %v\n", err)
		}
	}()
	switch s := p.(type) {
	default:
		// В default-переменная s имеет тип io.Writer, а не *bytes.Buffer.
		// Поэтому здесь снова видно "не nil interface", хотя внутри лежит nil-указатель.
		fmt.Printf("default, is nil = %t\n", s == nil) // false
	}
	fmt.Printf("write1: reflect.ValueOf(p)=%s\n", reflect.ValueOf(p))
	fmt.Printf("write1: reflect.ValueOf(p).IsNil()=%t\n", reflect.ValueOf(p).IsNil())
	if p != nil {
		// Это ключевая ловушка:
		// p != nil, потому что interface хранит пару (type=*bytes.Buffer, value=nil).
		fmt.Printf("write1: p is not nil\n")
		_, err := p.Write([]byte("qwe"))
		if err != nil {
			fmt.Printf("write1: err %v", err.Error())
			//log.Fatal(err.Error())
		}
	}
}

func write2(p *bytes.Buffer) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("write2: panic %v\n", err)
		}
	}()
	if p != nil {
		fmt.Printf("write2: p is not nil\n")
		_, err := p.Write([]byte("qwe"))
		if err != nil {
			fmt.Printf("write2: err %v", err.Error())
			//log.Fatal(err.Error())
		}
	}
	fmt.Println("empty") // сюда доходим, потому что у обычного pointer-параметра проверка p != nil работает ожидаемо
}
