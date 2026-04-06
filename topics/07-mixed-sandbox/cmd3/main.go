package main

import "fmt"

type X struct {
	V int
}

func (x X) S() {
	fmt.Println(x.V)
}

func main() {
	x := X{123}
	defer func() {
		x.S() // 456
	}()
	defer func(y X) {
		y.S() // 123
	}(x)
	x.V = 456
}
