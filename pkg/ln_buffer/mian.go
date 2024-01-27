package main

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
)

func main() {
	//buf := &bytes.Buffer{}
	var buf *bytes.Buffer

	write1(buf)
	write2(buf)
	if buf != nil {
		fmt.Println(buf)
	}
}

func write1(p io.Writer) {
	switch s := p.(type) {
	default:
		fmt.Println(s == nil)
	}
	fmt.Println(reflect.ValueOf(p))
	fmt.Println(reflect.ValueOf(p).IsNil())
	if p != nil {
		_, err := p.Write([]byte("qwe"))
		if err != nil {
			fmt.Println(err.Error())
			//log.Fatal(err.Error())
		}
	}
}

func write2(p *bytes.Buffer) {
	if p != nil {
		_, err := p.Write([]byte("qwe"))
		if err != nil {
			fmt.Println(err.Error())
			//log.Fatal(err.Error())
		}
	}
	fmt.Println("empty")
}
